package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer("mcp-google", "0.1.0")

	s.AddTool(
		mcp.NewTool("read_doc",
			mcp.WithDescription("Read the content of a Google Doc by document ID."),
			mcp.WithString("document_id",
				mcp.Required(),
				mcp.Description("The Google Docs document ID."),
			),
		),
		handleNotImplemented,
	)

	s.AddTool(
		mcp.NewTool("write_doc",
			mcp.WithDescription("Append or insert content into a Google Doc."),
			mcp.WithString("document_id",
				mcp.Required(),
				mcp.Description("The Google Docs document ID."),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Text content to insert."),
			),
			mcp.WithNumber("index",
				mcp.Description("Character index at which to insert. Appends to end if omitted."),
			),
		),
		handleNotImplemented,
	)

	s.AddTool(
		mcp.NewTool("read_sheet",
			mcp.WithDescription("Read data from a Google Sheets spreadsheet."),
			mcp.WithString("spreadsheet_id",
				mcp.Required(),
				mcp.Description("The Google Sheets spreadsheet ID."),
			),
			mcp.WithString("range",
				mcp.Required(),
				mcp.Description("A1 notation range to read, e.g. 'Sheet1!A1:D10'."),
			),
		),
		handleNotImplemented,
	)

	s.AddTool(
		mcp.NewTool("write_sheet",
			mcp.WithDescription("Write data to a Google Sheets spreadsheet."),
			mcp.WithString("spreadsheet_id",
				mcp.Required(),
				mcp.Description("The Google Sheets spreadsheet ID."),
			),
			mcp.WithString("range",
				mcp.Required(),
				mcp.Description("A1 notation range to write to, e.g. 'Sheet1!A1'."),
			),
			mcp.WithArray("values",
				mcp.Required(),
				mcp.Description("2D array of values to write (rows of columns)."),
				mcp.WithStringItems(),
			),
		),
		handleNotImplemented,
	)

	server.ServeStdio(s)
}

func handleNotImplemented(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError("not implemented yet"), nil
}
