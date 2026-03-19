package wizard

import (
	"regexp"
	"strings"
)

// EnvVarSpec describes an environment variable required by an MCP server.
type EnvVarSpec struct {
	Name        string
	Description string
	Required    bool
	Source      string // "dotenv" or "mcp_env"
	Default     string
}

// Dependency describes an external dependency required by an MCP server.
type Dependency struct {
	Name        string
	URL         string
	Description string
}

// mcpEnvRegistry maps MCP server binary names to their required env vars.
var mcpEnvRegistry = map[string][]EnvVarSpec{
	"mcp-email": {
		{Name: "SMTP_HOST", Description: "SMTP server hostname", Required: true, Source: "dotenv"},
		{Name: "SMTP_PORT", Description: "SMTP server port", Required: true, Source: "dotenv", Default: "587"},
		{Name: "SMTP_USER", Description: "SMTP username", Required: true, Source: "dotenv"},
		{Name: "SMTP_PASSWORD", Description: "SMTP password", Required: true, Source: "dotenv"},
		{Name: "SMTP_FROM", Description: "Sender email address", Required: true, Source: "dotenv"},
	},
	"mcp-telegram": {
		{Name: "TELEGRAM_BOT_TOKEN", Description: "Bot API token from @BotFather", Required: true, Source: "dotenv"},
		{Name: "TELEGRAM_CHAT_ID", Description: "Target chat/group ID", Required: true, Source: "dotenv"},
	},
	"mcp-whisper": {
		{Name: "WHISPER_BIN", Description: "Path to whisper.cpp binary", Required: true, Source: "dotenv"},
		{Name: "WHISPER_MODEL", Description: "Path to whisper model file (.bin)", Required: true, Source: "dotenv"},
		{Name: "WHISPER_MODELS_DIR", Description: "Directory containing whisper models", Required: false, Source: "dotenv"},
		{Name: "WHISPER_THREADS", Description: "Number of threads for whisper", Required: false, Source: "dotenv", Default: "4"},
	},
	"mcp-openapi": {
		{Name: "OPENAPI_SPEC_PATH", Description: "Path to OpenAPI spec file", Required: true, Source: "mcp_env"},
		{Name: "OPENAPI_BASE_URL", Description: "Override base URL from spec", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_TLS_INSECURE", Description: "Skip TLS cert verification (true/false)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_EXTRA_HEADERS", Description: "Extra HTTP headers (comma-separated key:value)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_AUTH_TYPE", Description: "Auth type: bearer, apikey, basic, oauth2, oauth2_client_credentials", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_AUTH_TOKEN", Description: "Bearer token for API auth", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_API_KEY", Description: "API key for apikey auth", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_BASIC_USER", Description: "Username for basic auth", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_BASIC_PASS", Description: "Password for basic auth", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_TOKEN_URL", Description: "OAuth2 token endpoint URL", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_CLIENT_ID", Description: "OAuth2 client ID", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_CLIENT_SECRET", Description: "OAuth2 client secret", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_REFRESH_URL", Description: "OAuth2 refresh token endpoint", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_ID_FIELD", Description: "Body field name for client ID (default: client_id)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_SECRET_FIELD", Description: "Body field name for secret (default: client_secret)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_GRANT_TYPE", Description: "Grant type value (empty to omit)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_TOKEN_IN", Description: "Token injection: header (default) or query", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_OAUTH2_TOKEN_PARAM", Description: "Query param name for token (default: access_token)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_INCLUDE_TAGS", Description: "Filter operations by tags (comma-separated)", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_INCLUDE_PATHS", Description: "Filter by path prefixes", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_INCLUDE_OPS", Description: "Include specific operationIds", Required: false, Source: "mcp_env"},
		{Name: "OPENAPI_EXCLUDE_OPS", Description: "Exclude specific operationIds", Required: false, Source: "mcp_env"},
	},
}

// mcpDepRegistry maps MCP server binary names to external dependencies.
var mcpDepRegistry = map[string][]Dependency{
	"mcp-whisper": {
		{Name: "whisper.cpp", URL: "https://github.com/ggerganov/whisper.cpp", Description: "Speech-to-text inference engine"},
		{Name: "ffmpeg", URL: "https://ffmpeg.org", Description: "Audio format conversion"},
	},
}

// envRefPattern matches ${VAR_NAME} references in strings.
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// extractEnvRefs parses ${VAR} references from a map of env values.
// Returns var names that are not already keys in the map (i.e. external references).
func extractEnvRefs(env map[string]string) []string {
	seen := make(map[string]bool)
	var refs []string
	for _, v := range env {
		matches := envRefPattern.FindAllStringSubmatch(v, -1)
		for _, m := range matches {
			varName := m[1]
			if !seen[varName] {
				seen[varName] = true
				refs = append(refs, varName)
			}
		}
	}
	return refs
}

// binaryName extracts the MCP server binary name from a command string.
// e.g. "./bin/mcp-openapi" → "mcp-openapi", "/usr/local/bin/mcp-email" → "mcp-email"
func binaryName(command string) string {
	parts := strings.Split(command, "/")
	return parts[len(parts)-1]
}
