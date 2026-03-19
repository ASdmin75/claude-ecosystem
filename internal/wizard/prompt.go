package wizard

import (
	"fmt"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// wizardJSONSchema constrains Claude's output to the Plan structure.
const wizardJSONSchema = `{
  "type": "object",
  "properties": {
    "summary": {
      "type": "string",
      "description": "Brief explanation of what this plan will create and why"
    },
    "mcp_servers": {
      "type": "array",
      "description": "New MCP server entries to add to config. Currently only mcp-openapi is supported for creation.",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string", "description": "kebab-case server name (e.g., crm-api, billing-api)" },
          "command": { "type": "string", "description": "Binary path, e.g. ./bin/mcp-openapi" },
          "args": { "type": "array", "items": { "type": "string" } },
          "env": {
            "type": "object",
            "description": "Environment variables for the server",
            "additionalProperties": { "type": "string" }
          }
        },
        "required": ["name", "command", "env"]
      }
    },
    "domains": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "description": { "type": "string" },
          "data_dir": { "type": "string" },
          "db": { "type": "string", "description": "Database filename only (e.g. 'data.db'), NOT a full path — data_dir is prepended automatically" },
          "schema": { "type": "string" },
          "domain_doc": { "type": "string" },
          "tasks": { "type": "array", "items": { "type": "string" } },
          "pipelines": { "type": "array", "items": { "type": "string" } },
          "agents": { "type": "array", "items": { "type": "string" } },
          "mcp_servers": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["name", "data_dir"]
      }
    },
    "agents": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "description": { "type": "string" },
          "tools": { "type": "array", "items": { "type": "string" } },
          "model": { "type": "string" },
          "permission_mode": { "type": "string" },
          "instructions": { "type": "string" },
          "scope": { "type": "string", "enum": ["user", "project"] }
        },
        "required": ["name", "description", "instructions"]
      }
    },
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "prompt": { "type": "string" },
          "work_dir": { "type": "string" },
          "schedule": { "type": "string" },
          "tags": { "type": "array", "items": { "type": "string" } },
          "model": { "type": "string" },
          "timeout": { "type": "string" },
          "agents": { "type": "array", "items": { "type": "string" } },
          "mcp_servers": { "type": "array", "items": { "type": "string" } },
          "allowed_tools": { "type": "array", "items": { "type": "string" } },
          "max_turns": { "type": "integer" },
          "max_budget_usd": { "type": "number" },
          "permission_mode": { "type": "string" },
          "domain": { "type": "string" }
        },
        "required": ["name", "prompt"]
      }
    },
    "pipelines": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "mode": { "type": "string", "enum": ["sequential", "parallel"] },
          "steps": { "type": "array", "items": { "type": "string" } },
          "max_iterations": { "type": "integer" },
          "stop_signal": { "type": "string" },
          "session_chain": { "type": "boolean", "description": "If true, pass session_id between sequential steps so each step resumes the previous conversation. All steps must share the same work_dir." }
        },
        "required": ["name", "steps"]
      }
    }
  },
    "setup_notes": {
      "type": "string",
      "description": "Human-readable notes about setup: external deps to install, API keys to obtain, services to configure. Optional."
    }
  },
  "required": ["summary", "tasks"]
}`

// buildWizardPrompt creates the system prompt for the wizard's Claude call.
func buildWizardPrompt(req GenerateRequest, cfg *config.Config) string {
	var sb strings.Builder

	sb.WriteString(`You are a configuration wizard for Claude Ecosystem — a Go orchestrator that runs Claude Code CLI as automated tasks.

Your job: given a user's natural language description, generate a structured JSON plan with the optimal set of entities (domains, agents, tasks, pipelines) to implement their request.

## Entity Types

### Tasks
The core unit of work. Each task runs "claude -p" with a prompt. Key fields:
- name: kebab-case identifier (e.g., "daily-code-review")
- prompt: Go template string — the actual prompt sent to Claude. Can use {{.Date}}, {{.PrevOutput}} in pipelines
- work_dir: working directory for task execution
- schedule: cron expression (e.g., "0 9 * * 1-5" for weekdays at 9am)
- model: Claude model to use (e.g., "sonnet", "opus", "haiku")
- timeout: duration string (e.g., "5m", "10m")
- agents: list of sub-agent names this task can delegate to
- mcp_servers: list of MCP server names for tool access
- allowed_tools: explicit tool allowlist. When using agents, include "Agent" in this list
- max_turns: limit on conversation turns (default unlimited)
- max_budget_usd: spending limit per execution
- permission_mode: "plan" (read-only), "default", or "full"
- domain: name of a domain this task operates on
- tags: categorization labels

### Sub-Agents
Specialist agents that tasks can delegate to via the "Agent" tool. Defined as .md files with YAML frontmatter.
- name: kebab-case identifier
- description: what this agent does (shown to Claude when choosing agents)
- tools: list of tools this agent can use
- model: optional model override
- permission_mode: agent's permission level
- instructions: markdown body with detailed instructions
- scope: "project" (default) or "user"

### Domains
Business data contexts with SQLite databases. Tasks reference a domain to get:
- Automatic DB schema initialization
- DOMAIN.md injected into system prompt
- Environment variables for MCP servers (DOMAIN_DB_PATH, DOMAIN_DATA_DIR)
Fields: name, data_dir, db (filename ONLY, e.g. "mydata.db" — do NOT include data_dir prefix), schema (CREATE TABLE SQL), domain_doc (filename ONLY for DOMAIN.md)

### Pipelines
Chain multiple tasks together.
- name: kebab-case identifier
- mode: "sequential" (default) or "parallel"
- steps: ordered list of task names
- max_iterations: loop count for sequential mode (default 10)
- stop_signal: text in output that stops iteration early
- session_chain: if true, each sequential step resumes the previous step's Claude session via --resume, preserving full conversation context. All steps MUST share the same work_dir. Only works with mode "sequential".

## MCP Tool Names

MCP tool names follow the pattern: mcp__{server}__{tool}. Here are the exact tool names:

- **database**: mcp__database__query, mcp__database__execute, mcp__database__list_tables, mcp__database__describe_table, mcp__database__check_exists, mcp__database__insert
- **excel**: mcp__excel__read_spreadsheet, mcp__excel__write_spreadsheet, mcp__excel__create_spreadsheet, mcp__excel__add_styled_table
- **email**: mcp__email__send_email, mcp__email__read_inbox, mcp__email__search_emails
- **telegram**: mcp__telegram__send_message, mcp__telegram__send_document
- **filesystem**: mcp__filesystem__read_file, mcp__filesystem__write_file, mcp__filesystem__list_directory, mcp__filesystem__search_files, mcp__filesystem__copy_file
- **openapi** (mcp-openapi): DYNAMIC tools — tool names are generated from the OpenAPI spec at startup. Pattern: mcp__{server_name}__{operationId_lowercase}. Example: if server is named "crm-api" and spec has operationId "getContacts", tool name is mcp__crm-api__getcontacts. Also provides a built-in mcp__{server_name}__download_file tool for downloading binary files (audio, images, etc.) to local paths.

## mcp-openapi Server

mcp-openapi dynamically generates MCP tools from an OpenAPI v2/v3 specification. Use it to integrate external REST APIs.

The wizard CAN create new mcp-openapi server entries in the plan's "mcp_servers" array. When the user wants to connect an external API, create an mcp-openapi entry with the appropriate env vars. The created server can then be referenced by tasks in the same plan.

Example mcp_servers entries in plan:

Simple bearer auth:
{"name": "crm-api", "command": "./bin/mcp-openapi", "env": {"OPENAPI_SPEC_PATH": "specs/crm.yaml", "OPENAPI_BASE_URL": "https://api.crm.example.com/v2", "OPENAPI_AUTH_TYPE": "bearer", "OPENAPI_AUTH_TOKEN": "${CRM_TOKEN}", "OPENAPI_INCLUDE_TAGS": "contacts,deals"}}

OAuth2 with custom body fields and token in query param (e.g. Yeastar PBX):
{"name": "yeastar-api", "command": "./bin/mcp-openapi", "env": {"OPENAPI_SPEC_PATH": "specs/yeastar.yaml", "OPENAPI_BASE_URL": "${YEASTAR_BASE_URL}/openapi/v1.0", "OPENAPI_AUTH_TYPE": "oauth2_client_credentials", "OPENAPI_OAUTH2_TOKEN_URL": "${YEASTAR_BASE_URL}/openapi/v1.0/get_token", "OPENAPI_OAUTH2_CLIENT_ID": "${YEASTAR_CLIENT_ID}", "OPENAPI_OAUTH2_CLIENT_SECRET": "${YEASTAR_CLIENT_SECRET}", "OPENAPI_OAUTH2_ID_FIELD": "username", "OPENAPI_OAUTH2_SECRET_FIELD": "password", "OPENAPI_OAUTH2_GRANT_TYPE": "", "OPENAPI_OAUTH2_TOKEN_IN": "query", "OPENAPI_OAUTH2_TOKEN_PARAM": "access_token", "OPENAPI_TLS_INSECURE": "true", "OPENAPI_EXTRA_HEADERS": "User-Agent:OpenAPI"}}

Environment variables for mcp-openapi:
**Core:**
- OPENAPI_SPEC_PATH (required): path to OpenAPI spec file (e.g. specs/myapi.yaml)
- OPENAPI_BASE_URL: override base URL from spec
- OPENAPI_TLS_INSECURE: "true" to skip TLS certificate verification (self-signed certs)
- OPENAPI_EXTRA_HEADERS: extra HTTP headers (comma-separated key:value pairs)
- OPENAPI_TIMEOUT: HTTP timeout (e.g. "60s"), default 30s

**Auth — simple types:**
- OPENAPI_AUTH_TYPE: "bearer", "apikey", "basic", "oauth2", or "oauth2_client_credentials"
- OPENAPI_AUTH_TOKEN: token for bearer auth
- OPENAPI_API_KEY, OPENAPI_API_KEY_NAME, OPENAPI_API_KEY_IN: for apikey auth (header or query)
- OPENAPI_BASIC_USER, OPENAPI_BASIC_PASS: for basic auth

**Auth — OAuth2 client credentials (auto token management with refresh):**
- OPENAPI_OAUTH2_TOKEN_URL (required): token endpoint URL
- OPENAPI_OAUTH2_CLIENT_ID (required): client ID / username
- OPENAPI_OAUTH2_CLIENT_SECRET (required): client secret / password
- OPENAPI_OAUTH2_REFRESH_URL: refresh token endpoint (falls back to re-auth if empty)
- OPENAPI_OAUTH2_ID_FIELD: body field name for client ID (default: "client_id"). Use "username" for APIs that expect username/password body.
- OPENAPI_OAUTH2_SECRET_FIELD: body field name for client secret (default: "client_secret"). Use "password" for APIs that expect username/password body.
- OPENAPI_OAUTH2_GRANT_TYPE: set to empty string "" to omit grant_type from body. Default includes "grant_type": "client_credentials".
- OPENAPI_OAUTH2_TOKEN_IN: "header" (default, Authorization: Bearer) or "query" (appends ?param=token)
- OPENAPI_OAUTH2_TOKEN_PARAM: query parameter name when TOKEN_IN=query (default: "access_token")

**Filtering:**
- OPENAPI_INCLUDE_TAGS: filter by tags (comma-separated)
- OPENAPI_INCLUDE_PATHS: filter by path prefixes
- OPENAPI_INCLUDE_OPS / OPENAPI_EXCLUDE_OPS: filter by operationId

When using OAuth2, the MCP server handles authentication transparently — Claude does NOT need to call auth endpoints or pass tokens manually. Do NOT include auth endpoints in the OpenAPI spec when using OAuth2. Auth is added to every request automatically.

Since mcp-openapi tools are dynamic, when adding allowed_tools for an openapi server, use the pattern mcp__{server_name}__{operationid_lowercase} based on the spec's operationId values. If you don't know exact operationIds, omit allowed_tools to allow all tools from that server.

IMPORTANT: Use ONLY these exact tool names (or the mcp-openapi pattern) in allowed_tools. Do NOT invent tool names.

## Permission Modes
- "plan" — read-only, no file writes or tool execution. Good for analysis tasks.
- "dontAsk" — REQUIRED for tasks that use MCP tools (database, excel, telegram, etc.) or agents. Without it, tool calls will be denied.
- "default" — interactive confirmation. NOT suitable for automated tasks.

## Rules
1. Use kebab-case for all names
2. Only reference MCP servers that exist in the current config OR are created in the same plan's mcp_servers array
3. If a task needs agents, create them and reference by name
4. If tasks share data, create a domain
5. Keep prompts focused and actionable
6. Set reasonable timeouts (5m default, more for complex tasks)
7. ALWAYS use permission_mode "dontAsk" for tasks with MCP tools or agents. Use "plan" only for pure read-only analysis.
8. For pipelines, ensure all referenced task names exist in the tasks array
9. Generate a clear summary explaining the plan
10. When using agents in allowed_tools, include "Agent" in the list
11. For tasks with MCP tools, list ALL MCP tools the task might need in allowed_tools

`)

	// Add available MCP servers
	if len(cfg.MCPServers) > 0 {
		sb.WriteString("## Available MCP Servers\n\n")
		for _, mcp := range cfg.MCPServers {
			sb.WriteString(fmt.Sprintf("- `%s`: %s %s\n", mcp.Name, mcp.Command, strings.Join(mcp.Args, " ")))
			// For mcp-openapi servers, show spec path and filters so wizard knows what APIs are available
			if strings.Contains(mcp.Command, "mcp-openapi") {
				for k, v := range mcp.Env {
					switch k {
					case "OPENAPI_SPEC_PATH", "OPENAPI_BASE_URL", "OPENAPI_AUTH_TYPE",
						"OPENAPI_INCLUDE_TAGS", "OPENAPI_INCLUDE_PATHS",
						"OPENAPI_OAUTH2_TOKEN_IN", "OPENAPI_TLS_INSECURE":
						sb.WriteString(fmt.Sprintf("    %s=%s\n", k, v))
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	// Add existing entity names to avoid conflicts
	if len(cfg.Tasks) > 0 {
		sb.WriteString("## Existing Tasks (avoid name conflicts)\n\n")
		for _, t := range cfg.Tasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t.Name))
		}
		sb.WriteString("\n")
	}

	if len(cfg.Pipelines) > 0 {
		sb.WriteString("## Existing Pipelines (avoid name conflicts)\n\n")
		for _, p := range cfg.Pipelines {
			sb.WriteString(fmt.Sprintf("- %s\n", p.Name))
		}
		sb.WriteString("\n")
	}

	if len(cfg.Domains) > 0 {
		sb.WriteString("## Existing Domains (avoid name conflicts)\n\n")
		for name := range cfg.Domains {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
		sb.WriteString("\n")
	}

	// Add user request
	sb.WriteString("## User Request\n\n")
	sb.WriteString(req.Description)
	sb.WriteString("\n")

	if req.WorkDir != "" {
		sb.WriteString(fmt.Sprintf("\nWorking directory: %s\n", req.WorkDir))
	}

	sb.WriteString("\n## Output Format\n\n")
	sb.WriteString("Respond with ONLY a raw JSON object (no markdown fences, no explanation, no extra text).\n")
	sb.WriteString("The JSON must match this schema:\n\n")
	sb.WriteString(wizardJSONSchema)
	sb.WriteString("\n\nIMPORTANT: Output ONLY the JSON object. No other text before or after it. Do NOT use any tools.")

	return sb.String()
}
