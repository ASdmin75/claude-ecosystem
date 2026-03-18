package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// parsePageRange parses a page range string into a slice of 1-based page numbers.
// Supports formats: "1-5", "1,3,7", "2", or empty string (all pages).
func parsePageRange(rangeStr string, totalPages int) ([]int, error) {
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		pages := make([]int, totalPages)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages, nil
	}

	pageSet := make(map[int]bool)
	parts := strings.Split(rangeStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid page range %q: %w", part, err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid page range %q: %w", part, err)
			}
			if start < 1 || end < 1 || start > totalPages || end > totalPages {
				return nil, fmt.Errorf("page range %q out of bounds (1-%d)", part, totalPages)
			}
			if start > end {
				return nil, fmt.Errorf("invalid page range %q: start > end", part)
			}
			for i := start; i <= end; i++ {
				pageSet[i] = true
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid page number %q: %w", part, err)
			}
			if num < 1 || num > totalPages {
				return nil, fmt.Errorf("page %d out of bounds (1-%d)", num, totalPages)
			}
			pageSet[num] = true
		}
	}

	pages := make([]int, 0, len(pageSet))
	for p := range pageSet {
		pages = append(pages, p)
	}
	// Sort pages in ascending order
	for i := 0; i < len(pages); i++ {
		for j := i + 1; j < len(pages); j++ {
			if pages[i] > pages[j] {
				pages[i], pages[j] = pages[j], pages[i]
			}
		}
	}
	return pages, nil
}

// extractPageText extracts text content from a single PDF page.
func extractPageText(p pdf.Page) string {
	rows, err := p.GetTextByRow()
	if err != nil || len(rows) == 0 {
		// Fallback: try plain content extraction
		content := p.Content()
		var buf strings.Builder
		for _, text := range content.Text {
			buf.WriteString(text.S)
		}
		return buf.String()
	}

	var buf strings.Builder
	for i, row := range rows {
		if i > 0 {
			buf.WriteString("\n")
		}
		for j, word := range row.Content {
			if j > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(word.S)
		}
	}
	return buf.String()
}

// detectTables attempts to identify tabular data in text.
// It looks for lines with consistent tab or multi-space delimiters.
func detectTables(text string) [][][]string {
	lines := strings.Split(text, "\n")
	var tables [][][]string
	var currentTable [][]string

	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			if len(currentTable) >= 2 {
				tables = append(tables, currentTable)
			}
			currentTable = nil
			continue
		}

		// Try to split by tab first
		cells := strings.Split(line, "\t")
		if len(cells) < 2 {
			// Try multi-space delimiter (2+ spaces)
			cells = splitByMultiSpace(line)
		}

		if len(cells) >= 2 {
			currentTable = append(currentTable, cells)
		} else {
			if len(currentTable) >= 2 {
				tables = append(tables, currentTable)
			}
			currentTable = nil
		}
	}

	if len(currentTable) >= 2 {
		tables = append(tables, currentTable)
	}

	// Filter: only keep tables where rows have a consistent column count
	var filtered [][][]string
	for _, table := range tables {
		if isConsistentTable(table) {
			filtered = append(filtered, table)
		}
	}

	return filtered
}

// splitByMultiSpace splits a line by runs of 2+ spaces.
func splitByMultiSpace(line string) []string {
	var cells []string
	var buf bytes.Buffer
	spaceCount := 0

	for _, ch := range line {
		if ch == ' ' {
			spaceCount++
			if spaceCount < 2 {
				buf.WriteRune(ch)
			}
		} else {
			if spaceCount >= 2 {
				cell := strings.TrimSpace(buf.String())
				if cell != "" {
					cells = append(cells, cell)
				}
				buf.Reset()
			}
			spaceCount = 0
			buf.WriteRune(ch)
		}
	}
	cell := strings.TrimSpace(buf.String())
	if cell != "" {
		cells = append(cells, cell)
	}
	return cells
}

// isConsistentTable checks if most rows have a similar column count.
func isConsistentTable(table [][]string) bool {
	if len(table) < 2 {
		return false
	}

	colCounts := make(map[int]int)
	for _, row := range table {
		colCounts[len(row)]++
	}

	maxCount := 0
	for _, count := range colCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	return maxCount >= len(table)/2
}

func handleReadPDF(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}
	pagesArg := req.GetString("pages", "")

	f, r, err := pdf.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open PDF: %s", err)), nil
	}
	defer f.Close()

	totalPages := r.NumPage()
	pageNums, err := parsePageRange(pagesArg, totalPages)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var textBuf strings.Builder
	for _, pageNum := range pageNums {
		p := r.Page(pageNum)
		if p.V.IsNull() {
			continue
		}
		pageText := extractPageText(p)
		if textBuf.Len() > 0 {
			textBuf.WriteString("\n")
		}
		fmt.Fprintf(&textBuf, "--- Page %d ---\n%s", pageNum, pageText)
	}

	info := fmt.Sprintf("File: %s\nTotal pages: %d\nPages read: %d\n\n%s",
		path, totalPages, len(pageNums), textBuf.String())

	return mcp.NewToolResultText(info), nil
}

func handleExtractText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}
	pagesArg := req.GetString("pages", "")

	// Check layout by looking at the raw arguments
	args := req.GetArguments()
	_, layoutRequested := args["layout"]

	f, r, err := pdf.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open PDF: %s", err)), nil
	}
	defer f.Close()

	totalPages := r.NumPage()
	pageNums, err := parsePageRange(pagesArg, totalPages)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var textBuf strings.Builder
	for _, pageNum := range pageNums {
		p := r.Page(pageNum)
		if p.V.IsNull() {
			continue
		}
		pageText := extractPageText(p)
		if textBuf.Len() > 0 {
			textBuf.WriteString("\n\n")
		}
		textBuf.WriteString(pageText)
	}

	result := textBuf.String()
	if layoutRequested {
		result = "Note: layout preservation is best-effort; this library returns plain text only.\n\n" + result
	}

	return mcp.NewToolResultText(result), nil
}

func handleExtractTables(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}
	pagesArg := req.GetString("pages", "")
	format := req.GetString("format", "json")

	f, r, err := pdf.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open PDF: %s", err)), nil
	}
	defer f.Close()

	totalPages := r.NumPage()
	pageNums, err := parsePageRange(pagesArg, totalPages)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type extractedTable struct {
		Page int        `json:"page"`
		Rows [][]string `json:"rows"`
	}

	var allTables []extractedTable

	for _, pageNum := range pageNums {
		p := r.Page(pageNum)
		if p.V.IsNull() {
			continue
		}

		pageText := extractPageText(p)
		tables := detectTables(pageText)
		for _, table := range tables {
			allTables = append(allTables, extractedTable{
				Page: pageNum,
				Rows: table,
			})
		}
	}

	if len(allTables) == 0 {
		return mcp.NewToolResultText("No tables detected in the specified pages."), nil
	}

	if format == "csv" {
		var buf strings.Builder
		for i, table := range allTables {
			if i > 0 {
				buf.WriteString("\n")
			}
			fmt.Fprintf(&buf, "# Table %d (Page %d)\n", i+1, table.Page)
			for _, row := range table.Rows {
				escaped := make([]string, len(row))
				for j, cell := range row {
					cell = strings.TrimSpace(cell)
					if strings.ContainsAny(cell, ",\"\n") {
						cell = "\"" + strings.ReplaceAll(cell, "\"", "\"\"") + "\""
					}
					escaped[j] = cell
				}
				buf.WriteString(strings.Join(escaped, ","))
				buf.WriteString("\n")
			}
		}
		return mcp.NewToolResultText(buf.String()), nil
	}

	// JSON format
	data, err := json.MarshalIndent(allTables, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal tables: %s", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func main() {
	s := server.NewMCPServer("mcp-pdf", "0.1.0")

	s.AddTool(
		mcp.NewTool("read_pdf",
			mcp.WithDescription("Read metadata and text content from a PDF file."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the PDF file."),
			),
			mcp.WithString("pages",
				mcp.Description("Page range to read, e.g. '1-5' or '1,3,7'. Reads all pages if omitted."),
			),
		),
		handleReadPDF,
	)

	s.AddTool(
		mcp.NewTool("extract_text",
			mcp.WithDescription("Extract plain text from a PDF file."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the PDF file."),
			),
			mcp.WithString("pages",
				mcp.Description("Page range to extract, e.g. '1-5'. Extracts all pages if omitted."),
			),
			mcp.WithBoolean("layout",
				mcp.Description("Whether to preserve spatial layout. Defaults to false."),
			),
		),
		handleExtractText,
	)

	s.AddTool(
		mcp.NewTool("extract_tables",
			mcp.WithDescription("Extract tabular data from a PDF file."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the PDF file."),
			),
			mcp.WithString("pages",
				mcp.Description("Page range to scan for tables, e.g. '1-5'. Scans all pages if omitted."),
			),
			mcp.WithString("format",
				mcp.Description("Output format for tables: 'json' or 'csv'. Defaults to 'json'."),
				mcp.Enum("json", "csv"),
			),
		),
		handleExtractTables,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("server error: %s\n", err)
	}
}
