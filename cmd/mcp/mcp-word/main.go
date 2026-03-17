package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var tools = []tool{
	{
		Name:        "read_document",
		Description: "Read the text content of a Word document.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Word document (.docx).",
				},
				"include_formatting": map[string]any{
					"type":        "boolean",
					"description": "Whether to include formatting metadata. Defaults to false.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "write_document",
		Description: "Append or replace content in an existing Word document.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Word document.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to write.",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Write mode: 'append' or 'replace'. Defaults to 'append'.",
					"enum":        []string{"append", "replace"},
				},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		Name:        "create_document",
		Description: "Create a new Word document with the given content.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path for the new Word document.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Document title.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Initial text content for the document.",
				},
			},
			"required": []string{"path"},
		},
	},
}

// XML structs for parsing docx word/document.xml

const nsW = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

type wDocument struct {
	XMLName xml.Name `xml:"document"`
	Body    wBody    `xml:"body"`
}

type wBody struct {
	Paragraphs []wParagraph `xml:"p"`
}

type wParagraph struct {
	PPr  *wPPr  `xml:"pPr"`
	Runs []wRun `xml:"r"`
}

type wPPr struct {
	PStyle *wPStyle `xml:"pStyle"`
}

type wPStyle struct {
	Val string `xml:"val,attr"`
}

type wRun struct {
	RPr  *wRPr  `xml:"rPr"`
	Text []wText `xml:"t"`
}

type wRPr struct {
	Bold   *struct{} `xml:"b"`
	Italic *struct{} `xml:"i"`
}

type wText struct {
	Space string `xml:"space,attr"`
	Value string `xml:",chardata"`
}

// XML templates for creating docx files

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const documentRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`

const documentXMLHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingml/2010/wordprocessingCanvas"
            xmlns:mo="http://schemas.microsoft.com/office/mac/office/2008/main"
            xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
            xmlns:mv="urn:schemas-microsoft-com:mac:vml"
            xmlns:o="urn:schemas-microsoft-com:office:office"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
            xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
            xmlns:v="urn:schemas-microsoft-com:vml"
            xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
            xmlns:w10="urn:schemas-microsoft-com:office:word"
            xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:wne="http://schemas.microsoft.com/office/word/2006/wordml"
            mc:Ignorable="w14 wp14">
  <w:body>`

const documentXMLFooter = `
  </w:body>
</w:document>`

func textResult(text string) any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func handleToolCall(params map[string]any) (any, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "read_document":
		return handleReadDocument(args)
	case "write_document":
		return handleWriteDocument(args)
	case "create_document":
		return handleCreateDocument(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// readDocxFile reads a .docx ZIP and returns the parsed document XML and all ZIP entries.
func readDocxFile(path string) (*wDocument, []*zipEntry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open docx: %w", err)
	}
	defer r.Close()

	var doc wDocument
	var entries []*zipEntry

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open zip entry %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read zip entry %s: %w", f.Name, err)
		}

		entries = append(entries, &zipEntry{
			Name: f.Name,
			Data: data,
		})

		if f.Name == "word/document.xml" {
			if err := xml.Unmarshal(data, &doc); err != nil {
				return nil, nil, fmt.Errorf("failed to parse document.xml: %w", err)
			}
		}
	}

	return &doc, entries, nil
}

type zipEntry struct {
	Name string
	Data []byte
}

// extractText extracts text from a parsed document, optionally with formatting annotations.
func extractText(doc *wDocument, includeFormatting bool) string {
	var paragraphs []string

	for _, p := range doc.Body.Paragraphs {
		var parts []string
		// Detect heading style
		headingStyle := ""
		if p.PPr != nil && p.PPr.PStyle != nil {
			style := p.PPr.PStyle.Val
			switch {
			case strings.EqualFold(style, "Heading1") || style == "1":
				headingStyle = "H1"
			case strings.EqualFold(style, "Heading2") || style == "2":
				headingStyle = "H2"
			case strings.EqualFold(style, "Heading3") || style == "3":
				headingStyle = "H3"
			case strings.HasPrefix(strings.ToLower(style), "heading"):
				headingStyle = "H" + strings.TrimPrefix(strings.ToLower(style), "heading")
			}
		}

		for _, r := range p.Runs {
			var text string
			for _, t := range r.Text {
				text += t.Value
			}
			if text == "" {
				continue
			}

			if includeFormatting {
				isBold := r.RPr != nil && r.RPr.Bold != nil
				isItalic := r.RPr != nil && r.RPr.Italic != nil
				if isBold {
					text = "[BOLD]" + text + "[/BOLD]"
				}
				if isItalic {
					text = "[ITALIC]" + text + "[/ITALIC]"
				}
			}
			parts = append(parts, text)
		}

		paraText := strings.Join(parts, "")
		if includeFormatting && headingStyle != "" && paraText != "" {
			paraText = "[" + headingStyle + "]" + paraText + "[/" + headingStyle + "]"
		}
		paragraphs = append(paragraphs, paraText)
	}

	return strings.Join(paragraphs, "\n")
}

// buildDocumentXML generates the word/document.xml content from paragraph data.
func buildDocumentXML(paragraphs []paragraphData) string {
	var buf strings.Builder
	buf.WriteString(documentXMLHeader)

	for _, p := range paragraphs {
		buf.WriteString("\n    <w:p>")
		if p.Style != "" {
			buf.WriteString(`<w:pPr><w:pStyle w:val="`)
			buf.WriteString(xmlEscape(p.Style))
			buf.WriteString(`"/></w:pPr>`)
		}
		buf.WriteString(`<w:r><w:t xml:space="preserve">`)
		buf.WriteString(xmlEscape(p.Text))
		buf.WriteString("</w:t></w:r>")
		buf.WriteString("</w:p>")
	}

	buf.WriteString(documentXMLFooter)
	return buf.String()
}

type paragraphData struct {
	Style string
	Text  string
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// writeDocxFromEntries writes a new ZIP file from the given entries, replacing word/document.xml.
func writeDocxFromEntries(path string, entries []*zipEntry, newDocXML string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, e := range entries {
		data := e.Data
		if e.Name == "word/document.xml" {
			data = []byte(newDocXML)
		}
		fw, err := zw.Create(e.Name)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", e.Name, err)
		}
		if _, err := fw.Write(data); err != nil {
			return fmt.Errorf("failed to write zip entry %s: %w", e.Name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to close zip: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// existingParagraphs extracts paragraph data from a parsed document.
func existingParagraphs(doc *wDocument) []paragraphData {
	var result []paragraphData
	for _, p := range doc.Body.Paragraphs {
		style := ""
		if p.PPr != nil && p.PPr.PStyle != nil {
			style = p.PPr.PStyle.Val
		}
		var text string
		for _, r := range p.Runs {
			for _, t := range r.Text {
				text += t.Value
			}
		}
		result = append(result, paragraphData{Style: style, Text: text})
	}
	return result
}

func handleReadDocument(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	includeFormatting, _ := args["include_formatting"].(bool)

	doc, _, err := readDocxFile(path)
	if err != nil {
		return nil, err
	}

	text := extractText(doc, includeFormatting)
	return textResult(text), nil
}

func handleWriteDocument(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "append"
	}

	doc, entries, err := readDocxFile(path)
	if err != nil {
		return nil, err
	}

	// Build new paragraphs from content
	lines := strings.Split(content, "\n")
	var newParas []paragraphData
	for _, line := range lines {
		newParas = append(newParas, paragraphData{Text: line})
	}

	var allParas []paragraphData
	if mode == "append" {
		allParas = append(existingParagraphs(doc), newParas...)
	} else {
		// replace
		allParas = newParas
	}

	docXML := buildDocumentXML(allParas)

	if err := writeDocxFromEntries(path, entries, docXML); err != nil {
		return nil, err
	}

	return textResult(fmt.Sprintf("Written %d paragraphs to %s (mode: %s)", len(newParas), path, mode)), nil
}

func handleCreateDocument(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	title, _ := args["title"].(string)
	content, _ := args["content"].(string)

	var paragraphs []paragraphData

	if title != "" {
		paragraphs = append(paragraphs, paragraphData{Style: "Heading1", Text: title})
	}

	if content != "" {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			paragraphs = append(paragraphs, paragraphData{Text: line})
		}
	}

	docXML := buildDocumentXML(paragraphs)

	entries := []*zipEntry{
		{Name: "[Content_Types].xml", Data: []byte(contentTypesXML)},
		{Name: "_rels/.rels", Data: []byte(relsXML)},
		{Name: "word/document.xml", Data: nil}, // placeholder, will be replaced
		{Name: "word/_rels/document.xml.rels", Data: []byte(documentRelsXML)},
	}

	if err := writeDocxFromEntries(path, entries, docXML); err != nil {
		return nil, err
	}

	paraCount := len(paragraphs)
	return textResult(fmt.Sprintf("Created %s with %d paragraphs", path, paraCount)), nil
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		var resp jsonRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo":     map[string]any{"name": "mcp-word", "version": "0.1.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = map[string]any{"code": -32602, "message": "invalid params: " + err.Error()}
			} else {
				result, err := handleToolCall(params)
				if err != nil {
					resp.Result = map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": "Error: " + err.Error()},
						},
						"isError": true,
					}
				} else {
					resp.Result = result
				}
			}
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}
