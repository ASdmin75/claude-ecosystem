package main

import (
	"bufio"
	"encoding/json"
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
		Name:        "read_file",
		Description: "Read the contents of a file at the given path.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to read.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "write_file",
		Description: "Write content to a file at the given path, creating or overwriting it.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file.",
				},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		Name:        "list_directory",
		Description: "List the contents of a directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the directory to list.",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "Whether to list recursively. Defaults to false.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "search_files",
		Description: "Search for files matching a pattern within a directory tree.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Root directory to search from.",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match file names against.",
				},
			},
			"required": []string{"path", "pattern"},
		},
	},
	{
		Name:        "copy_file",
		Description: "Copy a file from source to destination path.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"src": map[string]any{
					"type":        "string",
					"description": "Source file path.",
				},
				"dst": map[string]any{
					"type":        "string",
					"description": "Destination file path.",
				},
			},
			"required": []string{"src", "dst"},
		},
	},
}

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
	case "read_file":
		return handleReadFile(args)
	case "write_file":
		return handleWriteFile(args)
	case "list_directory":
		return handleListDirectory(args)
	case "search_files":
		return handleSearchFiles(args)
	case "copy_file":
		return handleCopyFile(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func handleReadFile(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return textResult(string(data)), nil
}

func handleWriteFile(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	return textResult(fmt.Sprintf("Written %d bytes to %s", len(content), path)), nil
}

func handleListDirectory(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	recursive, _ := args["recursive"].(bool)

	var entries []string
	if recursive {
		err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
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
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
		for _, e := range dirEntries {
			prefix := "FILE"
			if e.IsDir() {
				prefix = "DIR "
			}
			entries = append(entries, fmt.Sprintf("%s %s", prefix, e.Name()))
		}
	}

	return textResult(strings.Join(entries, "\n")), nil
}

func handleSearchFiles(args map[string]any) (any, error) {
	root, _ := args["path"].(string)
	pattern, _ := args["pattern"].(string)
	if root == "" || pattern == "" {
		return nil, fmt.Errorf("path and pattern are required")
	}

	var matches []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		matched, _ := filepath.Match(pattern, filepath.Base(p))
		if matched {
			matches = append(matches, p)
		}
		return nil
	})

	return textResult(strings.Join(matches, "\n")), nil
}

func handleCopyFile(args map[string]any) (any, error) {
	src, _ := args["src"].(string)
	dst, _ := args["dst"].(string)
	if src == "" || dst == "" {
		return nil, fmt.Errorf("src and dst are required")
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return nil, fmt.Errorf("failed to copy: %w", err)
	}

	return textResult(fmt.Sprintf("Copied %s to %s (%d bytes)", src, dst, n)), nil
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
				"serverInfo":     map[string]any{"name": "mcp-filesystem", "version": "0.1.0"},
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
