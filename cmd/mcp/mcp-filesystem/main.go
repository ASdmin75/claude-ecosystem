package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer("mcp-filesystem", "0.1.0")

	s.AddTool(mcp.NewTool("read_file",
		mcp.WithDescription("Read the contents of a file at the given path."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path to the file to read.")),
	), handleReadFile)

	s.AddTool(mcp.NewTool("write_file",
		mcp.WithDescription("Write content to a file at the given path, creating or overwriting it."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path to the file to write.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Content to write to the file.")),
	), handleWriteFile)

	s.AddTool(mcp.NewTool("list_directory",
		mcp.WithDescription("List the contents of a directory."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path to the directory to list.")),
		mcp.WithBoolean("recursive", mcp.Description("Whether to list recursively. Defaults to false.")),
	), handleListDirectory)

	s.AddTool(mcp.NewTool("search_files",
		mcp.WithDescription("Search for files matching a pattern within a directory tree."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Root directory to search from.")),
		mcp.WithString("pattern", mcp.Required(), mcp.Description("Glob pattern to match file names against.")),
	), handleSearchFiles)

	s.AddTool(mcp.NewTool("copy_file",
		mcp.WithDescription("Copy a file from source to destination path."),
		mcp.WithString("src", mcp.Required(), mcp.Description("Source file path.")),
		mcp.WithString("dst", mcp.Required(), mcp.Description("Destination file path.")),
	), handleCopyFile)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleReadFile(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func handleWriteFile(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	content := req.GetString("content", "")

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create directory: %v", err)), nil
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to write file: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Written %d bytes to %s", len(content), path)), nil
}

func handleListDirectory(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := req.GetArguments()
	recursive, _ := args["recursive"].(bool)

	var entries []string
	if recursive {
		err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(path, p)
			if rel == "." {
				return nil
			}
			prefix := "FILE"
			if info.IsDir() {
				prefix = "DIR "
			}
			entries = append(entries, fmt.Sprintf("%s %s", prefix, rel))
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to walk directory: %v", err)), nil
		}
	} else {
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read directory: %v", err)), nil
		}
		for _, e := range dirEntries {
			prefix := "FILE"
			if e.IsDir() {
				prefix = "DIR "
			}
			entries = append(entries, fmt.Sprintf("%s %s", prefix, e.Name()))
		}
	}

	return mcp.NewToolResultText(strings.Join(entries, "\n")), nil
}

func handleSearchFiles(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	root, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pattern, err := req.RequireString("pattern")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var matches []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		matched, _ := filepath.Match(pattern, filepath.Base(p))
		if matched {
			matches = append(matches, p)
		}
		return nil
	})

	return mcp.NewToolResultText(strings.Join(matches, "\n")), nil
}

func handleCopyFile(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	src, err := req.RequireString("src")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	dst, err := req.RequireString("dst")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open source: %v", err)), nil
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create destination directory: %v", err)), nil
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create destination: %v", err)), nil
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to copy: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Copied %s to %s (%d bytes)", src, dst, n)), nil
}
