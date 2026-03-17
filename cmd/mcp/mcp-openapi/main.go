package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// --- JSON-RPC types (same pattern as other MCP servers) ---

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

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// --- API operation model ---

type apiParameter struct {
	Name        string
	In          string // path, query, header
	Required    bool
	Description string
	Schema      map[string]any
}

type apiOperation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	Parameters  []apiParameter
	HasBody     bool
	BodySchema  map[string]any
}

type generatedTool struct {
	tool      mcpTool
	operation apiOperation
}

// --- Globals ---

var (
	generatedTools map[string]generatedTool
	toolList       []mcpTool
	baseURL        string
	httpClient     *http.Client
	authType       string
	authToken      string
	apiKey         string
	apiKeyName     string
	apiKeyIn       string
	basicUser      string
	basicPass      string
	extraHeaders   map[string]string
)

func main() {
	specPath := os.Getenv("OPENAPI_SPEC_PATH")
	if specPath == "" {
		fmt.Fprintf(os.Stderr, "OPENAPI_SPEC_PATH environment variable is required\n")
		os.Exit(1)
	}

	specData, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read spec file %s: %v\n", specPath, err)
		os.Exit(1)
	}

	// Parse OpenAPI spec
	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	model, err := doc.BuildV3Model()
	if err != nil && model == nil {
		fmt.Fprintf(os.Stderr, "failed to build OpenAPI v3 model: %v\n", err)
		os.Exit(1)
	}

	// Determine base URL
	baseURL = os.Getenv("OPENAPI_BASE_URL")
	if baseURL == "" {
		baseURL = extractBaseURL(model)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Extract and filter operations
	operations := extractOperations(model)
	operations = filterOperations(operations)

	// Apply max tools limit
	maxTools := 50
	if v := os.Getenv("OPENAPI_MAX_TOOLS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTools = n
		}
	}
	if len(operations) > maxTools {
		operations = operations[:maxTools]
	}

	// Generate tools
	generatedTools = make(map[string]generatedTool)
	toolList = nil
	nameCount := make(map[string]int)

	for _, op := range operations {
		name := toolName(op)
		nameCount[name]++
		if nameCount[name] > 1 {
			name = fmt.Sprintf("%s_%d", name, nameCount[name])
		}

		t := buildTool(name, op)
		generatedTools[name] = generatedTool{tool: t, operation: op}
		toolList = append(toolList, t)
	}

	// Configure HTTP client
	timeout := 30 * time.Second
	if v := os.Getenv("OPENAPI_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	httpClient = &http.Client{Timeout: timeout}

	// Configure auth
	authType = strings.ToLower(os.Getenv("OPENAPI_AUTH_TYPE"))
	authToken = os.Getenv("OPENAPI_AUTH_TOKEN")
	apiKey = os.Getenv("OPENAPI_API_KEY")
	apiKeyName = os.Getenv("OPENAPI_API_KEY_NAME")
	if apiKeyName == "" {
		apiKeyName = "X-API-Key"
	}
	apiKeyIn = strings.ToLower(os.Getenv("OPENAPI_API_KEY_IN"))
	if apiKeyIn == "" {
		apiKeyIn = "header"
	}
	basicUser = os.Getenv("OPENAPI_BASIC_USER")
	basicPass = os.Getenv("OPENAPI_BASIC_PASS")

	// Parse extra headers
	extraHeaders = parseExtraHeaders(os.Getenv("OPENAPI_EXTRA_HEADERS"))

	fmt.Fprintf(os.Stderr, "mcp-openapi: loaded %d tools from %s (base: %s)\n", len(toolList), specPath, baseURL)

	// JSON-RPC main loop
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
				"serverInfo":     map[string]any{"name": "mcp-openapi", "version": "1.0.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": toolList}
		case "tools/call":
			resp.Result = handleToolCall(req.Params)
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}

// --- Spec parsing ---

func extractBaseURL(model *libopenapi.DocumentModel[v3.Document]) string {
	if len(model.Model.Servers) > 0 {
		return model.Model.Servers[0].URL
	}
	return "http://localhost"
}

func extractOperations(model *libopenapi.DocumentModel[v3.Document]) []apiOperation {
	var ops []apiOperation

	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return ops
	}

	for pair := orderedmap.First(model.Model.Paths.PathItems); pair != nil; pair = pair.Next() {
		path := pair.Key()
		item := pair.Value()

		methods := map[string]*v3.Operation{
			"GET":     item.Get,
			"POST":    item.Post,
			"PUT":     item.Put,
			"DELETE":  item.Delete,
			"PATCH":   item.Patch,
			"HEAD":    item.Head,
			"OPTIONS": item.Options,
		}

		// Collect path-level parameters
		var pathParams []apiParameter
		if item.Parameters != nil {
			for _, p := range item.Parameters {
				pathParams = append(pathParams, convertParameter(p))
			}
		}

		for method, op := range methods {
			if op == nil {
				continue
			}

			apiOp := apiOperation{
				Method:  method,
				Path:    path,
				Summary: op.Summary,
			}

			if op.Description != "" {
				apiOp.Description = op.Description
			}

			if op.OperationId != "" {
				apiOp.OperationID = op.OperationId
			}

			if op.Tags != nil {
				apiOp.Tags = op.Tags
			}

			// Merge path-level + operation-level params
			apiOp.Parameters = append(apiOp.Parameters, pathParams...)
			if op.Parameters != nil {
				for _, p := range op.Parameters {
					apiOp.Parameters = append(apiOp.Parameters, convertParameter(p))
				}
			}

			// Request body
			if op.RequestBody != nil && op.RequestBody.Content != nil {
				jsonContent := findJSONContent(op.RequestBody.Content)
				if jsonContent != nil && jsonContent.Schema != nil {
					apiOp.HasBody = true
					apiOp.BodySchema = schemaToMap(jsonContent.Schema.Schema(), 0)
				}
			}

			ops = append(ops, apiOp)
		}
	}

	return ops
}

func convertParameter(p *v3.Parameter) apiParameter {
	ap := apiParameter{
		Name:        p.Name,
		In:          p.In,
		Required:    p.Required != nil && *p.Required,
		Description: p.Description,
	}
	if p.Schema != nil {
		ap.Schema = schemaToMap(p.Schema.Schema(), 0)
	}
	return ap
}

func findJSONContent(content *orderedmap.Map[string, *v3.MediaType]) *v3.MediaType {
	for pair := orderedmap.First(content); pair != nil; pair = pair.Next() {
		if strings.Contains(pair.Key(), "json") {
			return pair.Value()
		}
	}
	return nil
}

const maxSchemaDepth = 3

func schemaToMap(schema *base.Schema, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth {
		return map[string]any{"type": "object"}
	}

	result := make(map[string]any)

	if len(schema.Type) > 0 {
		result["type"] = schema.Type[0]
	}
	if schema.Description != "" {
		result["description"] = schema.Description
	}
	if len(schema.Enum) > 0 {
		enums := make([]any, len(schema.Enum))
		for i, e := range schema.Enum {
			enums[i] = e.Value
		}
		result["enum"] = enums
	}

	// Object properties
	if schema.Properties != nil && orderedmap.Len(schema.Properties) > 0 {
		props := make(map[string]any)
		for pair := orderedmap.First(schema.Properties); pair != nil; pair = pair.Next() {
			if pair.Value() != nil {
				props[pair.Key()] = schemaToMap(pair.Value().Schema(), depth+1)
			}
		}
		result["properties"] = props
	}

	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Array items
	if schema.Items != nil && schema.Items.IsA() {
		result["items"] = schemaToMap(schema.Items.A.Schema(), depth+1)
	}

	return result
}

// --- Filtering ---

func filterOperations(ops []apiOperation) []apiOperation {
	includeTags := parseCSV(os.Getenv("OPENAPI_INCLUDE_TAGS"))
	includePaths := parseCSV(os.Getenv("OPENAPI_INCLUDE_PATHS"))
	includeOps := parseCSV(os.Getenv("OPENAPI_INCLUDE_OPS"))
	excludeOps := parseCSV(os.Getenv("OPENAPI_EXCLUDE_OPS"))

	if len(includeTags) == 0 && len(includePaths) == 0 && len(includeOps) == 0 && len(excludeOps) == 0 {
		return ops
	}

	var filtered []apiOperation
	for _, op := range ops {
		if len(excludeOps) > 0 && containsStr(excludeOps, op.OperationID) {
			continue
		}
		if len(includeOps) > 0 {
			if containsStr(includeOps, op.OperationID) {
				filtered = append(filtered, op)
			}
			continue
		}
		if len(includeTags) > 0 {
			if !hasAnyTag(op.Tags, includeTags) {
				continue
			}
		}
		if len(includePaths) > 0 {
			if !hasPathPrefix(op.Path, includePaths) {
				continue
			}
		}
		filtered = append(filtered, op)
	}
	return filtered
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func hasAnyTag(tags []string, include []string) bool {
	for _, t := range tags {
		if containsStr(include, t) {
			return true
		}
	}
	return false
}

func hasPathPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// --- Tool name generation ---

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func toolName(op apiOperation) string {
	if op.OperationID != "" {
		name := strings.ToLower(op.OperationID)
		name = nonAlnum.ReplaceAllString(name, "_")
		return strings.Trim(name, "_")
	}
	// method_path_segments
	path := strings.ToLower(op.Path)
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = nonAlnum.ReplaceAllString(path, "_")
	path = strings.Trim(path, "_")
	return strings.ToLower(op.Method) + "_" + path
}

// --- Tool schema generation ---

func buildTool(name string, op apiOperation) mcpTool {
	desc := op.Summary
	if desc == "" {
		desc = op.Description
	}
	if desc == "" {
		desc = fmt.Sprintf("%s %s", op.Method, op.Path)
	}
	if len(desc) > 500 {
		desc = desc[:497] + "..."
	}

	properties := make(map[string]any)
	var required []string

	for _, p := range op.Parameters {
		propSchema := map[string]any{"type": "string"}
		if p.Schema != nil {
			propSchema = p.Schema
		}
		if p.Description != "" {
			propSchema["description"] = p.Description
		}

		propName := p.Name
		if p.In == "header" {
			propName = "header_" + p.Name
		}

		properties[propName] = propSchema
		if p.Required {
			required = append(required, propName)
		}
	}

	if op.HasBody && op.BodySchema != nil {
		bodyDesc := "Request body (JSON)"
		if d, ok := op.BodySchema["description"].(string); ok && d != "" {
			bodyDesc = d
		}
		bodyProp := copyMap(op.BodySchema)
		bodyProp["description"] = bodyDesc
		properties["body"] = bodyProp
	}

	inputSchema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		inputSchema["required"] = required
	}

	return mcpTool{
		Name:        name,
		Description: desc,
		InputSchema: inputSchema,
	}
}

func copyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// --- Tool execution ---

func handleToolCall(params json.RawMessage) toolResult {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResult("invalid params: " + err.Error())
	}

	gt, ok := generatedTools[p.Name]
	if !ok {
		return errorResult("unknown tool: " + p.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(p.Arguments, &args); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	return executeOperation(gt.operation, args)
}

func executeOperation(op apiOperation, args map[string]any) toolResult {
	// Build URL with path params
	reqPath := op.Path
	queryParams := url.Values{}

	for _, p := range op.Parameters {
		argName := p.Name
		if p.In == "header" {
			argName = "header_" + p.Name
		}

		val, exists := args[argName]
		if !exists {
			continue
		}
		strVal := fmt.Sprintf("%v", val)

		switch p.In {
		case "path":
			reqPath = strings.ReplaceAll(reqPath, "{"+p.Name+"}", url.PathEscape(strVal))
		case "query":
			queryParams.Set(p.Name, strVal)
		}
	}

	fullURL := baseURL + reqPath
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	// Build body
	var bodyReader io.Reader
	if op.HasBody {
		if bodyData, ok := args["body"]; ok {
			bodyBytes, err := json.Marshal(bodyData)
			if err != nil {
				return errorResult("failed to marshal body: " + err.Error())
			}
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	// Create request
	req, err := http.NewRequest(op.Method, fullURL, bodyReader)
	if err != nil {
		return errorResult("failed to create request: " + err.Error())
	}

	if op.HasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Apply auth
	applyAuth(req)

	// Apply header params from args
	for _, p := range op.Parameters {
		if p.In == "header" {
			if val, ok := args["header_"+p.Name]; ok {
				req.Header.Set(p.Name, fmt.Sprintf("%v", val))
			}
		}
	}

	// Apply extra headers
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	// Execute
	resp, err := httpClient.Do(req)
	if err != nil {
		return errorResult("HTTP request failed: " + err.Error())
	}
	defer resp.Body.Close()

	// Read response (limit 1MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return errorResult("failed to read response: " + err.Error())
	}

	// Format response
	var output string
	if isJSON(body) {
		var buf bytes.Buffer
		json.Indent(&buf, body, "", "  ")
		output = fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, buf.String())
	} else {
		output = fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= 400 {
		return toolResult{
			Content: []contentItem{{Type: "text", Text: output}},
			IsError: true,
		}
	}

	return textResult(output)
}

func applyAuth(req *http.Request) {
	switch authType {
	case "bearer":
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
	case "apikey":
		if apiKey != "" {
			if apiKeyIn == "query" {
				q := req.URL.Query()
				q.Set(apiKeyName, apiKey)
				req.URL.RawQuery = q.Encode()
			} else {
				req.Header.Set(apiKeyName, apiKey)
			}
		}
	case "basic":
		if basicUser != "" {
			cred := base64.StdEncoding.EncodeToString([]byte(basicUser + ":" + basicPass))
			req.Header.Set("Authorization", "Basic "+cred)
		}
	}
}

func parseExtraHeaders(s string) map[string]string {
	headers := make(map[string]string)
	if s == "" {
		return headers
	}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

func isJSON(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && (data[0] == '{' || data[0] == '[')
}

func textResult(text string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}
