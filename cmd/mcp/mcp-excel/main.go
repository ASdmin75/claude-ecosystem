package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xuri/excelize/v2"
)

func main() {
	s := server.NewMCPServer("mcp-excel", "0.1.0")

	s.AddTool(mcp.NewTool("read_spreadsheet",
		mcp.WithDescription("Read data from an Excel spreadsheet."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the Excel file (.xlsx or .xls).")),
		mcp.WithString("sheet", mcp.Description("Name of the sheet to read. Defaults to the first sheet.")),
		mcp.WithString("range", mcp.Description("Cell range to read, e.g. 'A1:D10'. Reads entire sheet if omitted.")),
	), handleReadSpreadsheet)

	s.AddTool(mcp.NewTool("write_spreadsheet",
		mcp.WithDescription("Write data to an existing Excel spreadsheet."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the Excel file.")),
		mcp.WithString("sheet", mcp.Required(), mcp.Description("Name of the sheet to write to.")),
		mcp.WithString("cell", mcp.Required(), mcp.Description("Starting cell for the write, e.g. 'A1'.")),
		mcp.WithArray("data", mcp.Required(), mcp.Description("2D array of values to write (rows of columns).")),
	), handleWriteSpreadsheet)

	s.AddTool(mcp.NewTool("create_spreadsheet",
		mcp.WithDescription("Create a new Excel spreadsheet with optional initial data."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path for the new Excel file.")),
		mcp.WithString("sheet", mcp.Description("Name of the initial sheet. Defaults to 'Sheet1'.")),
		mcp.WithArray("headers", mcp.Description("Column headers for the first row."), mcp.WithStringItems()),
	), handleCreateSpreadsheet)

	s.AddTool(mcp.NewTool("add_styled_table",
		mcp.WithDescription("Add a styled table with auto-filter, header styling, and alternating row colors to a sheet."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the Excel file.")),
		mcp.WithString("sheet", mcp.Required(), mcp.Description("Name of the sheet to add the table to.")),
		mcp.WithArray("headers", mcp.Required(), mcp.Description("Column headers."), mcp.WithStringItems()),
		mcp.WithArray("data", mcp.Required(), mcp.Description("2D array of row data.")),
		mcp.WithString("table_name", mcp.Description("Name for the table. Defaults to 'Table1'.")),
	), handleAddStyledTable)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleReadSpreadsheet(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open file: %v", err)), nil
	}
	defer f.Close()

	sheet := req.GetString("sheet", "")
	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	var rows [][]string
	rangeStr := req.GetString("range", "")
	if rangeStr != "" {
		// Parse range like "A1:D10"
		parts := strings.SplitN(rangeStr, ":", 2)
		if len(parts) != 2 {
			return mcp.NewToolResultError("invalid range format, expected 'A1:D10'"), nil
		}
		startCol, startRow, err := excelize.CellNameToCoordinates(parts[0])
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid start cell: %v", err)), nil
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(parts[1])
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid end cell: %v", err)), nil
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
			return mcp.NewToolResultError(fmt.Sprintf("failed to get rows: %v", err)), nil
		}
	}

	data, err := json.Marshal(rows)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal rows: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func handleWriteSpreadsheet(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}
	sheet, err := req.RequireString("sheet")
	if err != nil {
		return mcp.NewToolResultError("sheet is required"), nil
	}
	cell, err := req.RequireString("cell")
	if err != nil {
		return mcp.NewToolResultError("cell is required"), nil
	}

	args := req.GetArguments()
	dataRaw, ok := args["data"].([]any)
	if !ok || len(dataRaw) == 0 {
		return mcp.NewToolResultError("data is required and must be a non-empty array"), nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open file: %v", err)), nil
	}
	defer f.Close()

	startCol, startRow, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid cell: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to save file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Written %d rows to %s!%s starting at %s", len(dataRaw), path, sheet, cell)), nil
}

func handleCreateSpreadsheet(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	sheet := req.GetString("sheet", "Sheet1")

	f := excelize.NewFile()
	defer f.Close()

	// Rename default sheet
	defaultSheet := f.GetSheetName(0)
	if defaultSheet != sheet {
		idx, _ := f.GetSheetIndex(defaultSheet)
		f.SetSheetName(f.GetSheetName(idx), sheet)
	}

	// Write headers if provided
	args := req.GetArguments()
	if headersRaw, ok := args["headers"].([]any); ok && len(headersRaw) > 0 {
		for i, h := range headersRaw {
			cellName, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cellName, h)
		}
	}

	if err := f.SaveAs(path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Created spreadsheet: %s (sheet: %s)", path, sheet)), nil
}

func handleAddStyledTable(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}
	sheet, err := req.RequireString("sheet")
	if err != nil {
		return mcp.NewToolResultError("sheet is required"), nil
	}

	args := req.GetArguments()
	headersRaw, ok := args["headers"].([]any)
	if !ok || len(headersRaw) == 0 {
		return mcp.NewToolResultError("headers is required and must be a non-empty array"), nil
	}
	dataRaw, ok := args["data"].([]any)
	if !ok {
		return mcp.NewToolResultError("data is required and must be an array"), nil
	}

	tableName := req.GetString("table_name", "Table1")

	var f *excelize.File

	if _, statErr := os.Stat(path); statErr == nil {
		f, err = excelize.OpenFile(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open file: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to save file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Added styled table '%s' to %s!%s with %d columns and %d data rows",
		tableName, path, sheet, numCols, len(dataRaw))), nil
}
