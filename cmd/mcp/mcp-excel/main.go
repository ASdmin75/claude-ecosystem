package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
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
		Name:        "read_spreadsheet",
		Description: "Read data from an Excel spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Excel file (.xlsx or .xls).",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the sheet to read. Defaults to the first sheet.",
				},
				"range": map[string]any{
					"type":        "string",
					"description": "Cell range to read, e.g. 'A1:D10'. Reads entire sheet if omitted.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "write_spreadsheet",
		Description: "Write data to an existing Excel spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Excel file.",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the sheet to write to.",
				},
				"cell": map[string]any{
					"type":        "string",
					"description": "Starting cell for the write, e.g. 'A1'.",
				},
				"data": map[string]any{
					"type":        "array",
					"description": "2D array of values to write (rows of columns).",
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
			"required": []string{"path", "sheet", "cell", "data"},
		},
	},
	{
		Name:        "create_spreadsheet",
		Description: "Create a new Excel spreadsheet with optional initial data.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path for the new Excel file.",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the initial sheet. Defaults to 'Sheet1'.",
				},
				"headers": map[string]any{
					"type":        "array",
					"description": "Column headers for the first row.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "add_styled_table",
		Description: "Add a styled table with auto-filter, header styling, and alternating row colors to a sheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Excel file.",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the sheet to add the table to.",
				},
				"headers": map[string]any{
					"type":        "array",
					"description": "Column headers.",
					"items":       map[string]any{"type": "string"},
				},
				"data": map[string]any{
					"type":        "array",
					"description": "2D array of row data.",
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name for the table. Defaults to 'Table1'.",
				},
			},
			"required": []string{"path", "sheet", "headers", "data"},
		},
	},
}

func handleToolCall(params map[string]any) (any, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "read_spreadsheet":
		return handleReadSpreadsheet(args)
	case "write_spreadsheet":
		return handleWriteSpreadsheet(args)
	case "create_spreadsheet":
		return handleCreateSpreadsheet(args)
	case "add_styled_table":
		return handleAddStyledTable(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func textResult(text string) any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func handleReadSpreadsheet(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	sheet, _ := args["sheet"].(string)
	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	var rows [][]string
	if rangeStr, ok := args["range"].(string); ok && rangeStr != "" {
		// Parse range like "A1:D10"
		parts := strings.SplitN(rangeStr, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format, expected 'A1:D10'")
		}
		startCol, startRow, err := excelize.CellNameToCoordinates(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid start cell: %w", err)
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid end cell: %w", err)
		}
		for r := startRow; r <= endRow; r++ {
			var row []string
			for c := startCol; c <= endCol; c++ {
				cellName, _ := excelize.CoordinatesToCellName(c, r)
				val, _ := f.GetCellValue(sheet, cellName)
				row = append(row, val)
			}
			rows = append(rows, row)
		}
	} else {
		rows, err = f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("failed to get rows: %w", err)
		}
	}

	data, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rows: %w", err)
	}
	return textResult(string(data)), nil
}

func handleWriteSpreadsheet(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	sheet, _ := args["sheet"].(string)
	cell, _ := args["cell"].(string)
	dataRaw, _ := args["data"].([]any)

	if path == "" || sheet == "" || cell == "" {
		return nil, fmt.Errorf("path, sheet, and cell are required")
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	startCol, startRow, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return nil, fmt.Errorf("invalid cell: %w", err)
	}

	for ri, rowRaw := range dataRaw {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		for ci, val := range row {
			cellName, _ := excelize.CoordinatesToCellName(startCol+ci, startRow+ri)
			f.SetCellValue(sheet, cellName, val)
		}
	}

	if err := f.SaveAs(path); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	return textResult(fmt.Sprintf("Written %d rows to %s!%s starting at %s", len(dataRaw), path, sheet, cell)), nil
}

func handleCreateSpreadsheet(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	sheet, _ := args["sheet"].(string)
	if sheet == "" {
		sheet = "Sheet1"
	}

	f := excelize.NewFile()
	defer f.Close()

	// Rename default sheet
	defaultSheet := f.GetSheetName(0)
	if defaultSheet != sheet {
		idx, _ := f.GetSheetIndex(defaultSheet)
		f.SetSheetName(f.GetSheetName(idx), sheet)
	}

	// Write headers if provided
	if headersRaw, ok := args["headers"].([]any); ok && len(headersRaw) > 0 {
		for i, h := range headersRaw {
			cellName, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cellName, h)
		}
	}

	if err := f.SaveAs(path); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	return textResult(fmt.Sprintf("Created spreadsheet: %s (sheet: %s)", path, sheet)), nil
}

func handleAddStyledTable(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	sheet, _ := args["sheet"].(string)
	headersRaw, _ := args["headers"].([]any)
	dataRaw, _ := args["data"].([]any)
	tableName, _ := args["table_name"].(string)

	if path == "" || sheet == "" || len(headersRaw) == 0 {
		return nil, fmt.Errorf("path, sheet, and headers are required")
	}
	if tableName == "" {
		tableName = "Table1"
	}

	var f *excelize.File
	var err error

	if _, statErr := os.Stat(path); statErr == nil {
		f, err = excelize.OpenFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
	} else {
		f = excelize.NewFile()
	}
	defer f.Close()

	// Ensure sheet exists
	if idx, _ := f.GetSheetIndex(sheet); idx < 0 {
		f.NewSheet(sheet)
	}

	numCols := len(headersRaw)
	numRows := len(dataRaw) + 1 // +1 for header

	// Write headers with bold style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "2F5496", Style: 2},
		},
	})

	for i, h := range headersRaw {
		cellName, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cellName, h)
		f.SetCellStyle(sheet, cellName, cellName, headerStyle)
	}

	// Alternating row styles
	evenStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D6E4F0"}},
	})
	oddStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF"}},
	})

	// Write data rows
	for ri, rowRaw := range dataRaw {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		style := oddStyle
		if ri%2 == 0 {
			style = evenStyle
		}
		for ci, val := range row {
			cellName, _ := excelize.CoordinatesToCellName(ci+1, ri+2) // +2: 1-indexed + header
			f.SetCellValue(sheet, cellName, val)
			f.SetCellStyle(sheet, cellName, cellName, style)
		}
	}

	// Add auto-filter
	endCell, _ := excelize.CoordinatesToCellName(numCols, numRows)
	if err := f.AutoFilter(sheet, "A1:"+endCell, nil); err != nil {
		// Non-fatal, continue
		_ = err
	}

	// Auto-fit column widths (approximate)
	for i := range headersRaw {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, colName, colName, 18)
	}

	if err := f.SaveAs(path); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	return textResult(fmt.Sprintf("Added styled table '%s' to %s!%s with %d columns and %d data rows",
		tableName, path, sheet, numCols, len(dataRaw))), nil
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
				"serverInfo":     map[string]any{"name": "mcp-excel", "version": "0.1.0"},
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
