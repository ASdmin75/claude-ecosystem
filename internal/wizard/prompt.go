package wizard

import (
	"fmt"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
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
func buildWizardPrompt(req GenerateRequest, cfg *config.Config, agents []subagent.SubAgent) string {
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
- permission_mode: "dontAsk" (REQUIRED for tasks with MCP tools or agents — allows all tool calls without prompting), "plan" (read-only, no file writes or tool execution), "default" (interactive, NOT for automation), or "full" (all permissions)
- domain: name of a domain this task operates on
- tags: categorization labels

### Sub-Agents
Specialist agents that tasks can delegate to via the "Agent" tool. Defined as .md files with YAML frontmatter.
- name: kebab-case identifier
- description: what this agent does (shown to Claude when choosing agents)
- tools: list of tools this agent can use
- model: optional model override
- permission_mode: "dontAsk" (REQUIRED when agent uses MCP tools like database, excel, etc.), "plan" (read-only), or "default"
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

Since mcp-openapi tools are dynamic, when adding allowed_tools for an openapi server, use the pattern mcp__{server_name}__{operationid_lowercase} based on the spec's operationId values.

CRITICAL — openapi safety: Tasks using openapi servers connect to EXTERNAL systems (CRMs, ERPs, APIs). Unlike local MCP servers (database, excel, filesystem), openapi calls can modify production data irreversibly. Therefore:
- ALWAYS specify explicit allowed_tools for openapi servers — list only the endpoints the task actually needs
- NEVER omit allowed_tools for tasks with openapi servers — this would give Claude unrestricted access to all API endpoints including destructive ones (DELETE, PUT, POST that create/modify records)
- Prefer read-only endpoints (GET/list) unless the task explicitly needs to write data
- If the user's description is ambiguous about which endpoints are needed, include only read/list endpoints and add a warning

IMPORTANT: Use ONLY these exact tool names (or the mcp-openapi pattern) in allowed_tools. Do NOT invent tool names.

## Permission Modes
- "dontAsk" — REQUIRED for tasks AND sub-agents that use MCP tools (database, excel, telegram, etc.) or agents. Without it, tool calls will be denied and the model will loop until max_turns. NOTE: dontAsk allows calling ANY tool in allowed_tools without confirmation, so use allowed_tools as the safety whitelist — especially for openapi servers that access external systems.
- "plan" — read-only, no file writes or tool execution. Good for pure analysis tasks that only read code.
- "default" — interactive confirmation. NOT suitable for automated tasks.
- "full" — all permissions including file system writes. Use when task needs to write files directly (not via MCP).

Safety model: permission_mode controls WHETHER tools can be called. allowed_tools controls WHICH tools can be called. For external APIs (openapi), always combine "dontAsk" + strict allowed_tools whitelist.

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
11. For tasks with MCP tools, list ALL MCP tools the task might need in allowed_tools. ALWAYS include discovery/utility tools for each server — these are essential for the model to understand the data structure:
   - database: ALWAYS include mcp__database__list_tables and mcp__database__describe_table (models call these first to understand the schema before running queries)
   - filesystem: ALWAYS include mcp__filesystem__list_directory
   - excel: ALWAYS include mcp__excel__read_spreadsheet when writing spreadsheets
12. When the user asks to create something "like" or "similar to" an existing entity, use the full details provided below to clone and adapt it. Preserve structural patterns (MCP servers, agents, domain setup) unless the user explicitly requests changes.
13. CRITICAL — stop_signal safety: if a pipeline has a stop_signal, ONLY the LAST step's prompt may instruct Claude to output that signal. Non-final step prompts must NEVER mention or output the stop_signal text, because the pipeline engine checks for it after EVERY step and will skip remaining steps if found early. If the pipeline has max_iterations: 1 and no iterative review loop, prefer omitting stop_signal entirely.
14. Pipeline data flow: in sequential pipelines, {{.PrevOutput}} carries the text output from the previous step. Each step MUST output all data that the next step needs (IDs, counts, file paths, summaries). NEVER instruct a step to "return only the file path" — this strips context that later steps depend on. Delivery/report steps should extract all needed data from {{.PrevOutput}} text, NOT by reading binary files (xlsx, pdf, etc.) which filesystem tools cannot parse. Binary files should only be referenced by path for sending (e.g., telegram send_document).

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

	// Add existing entities with full details for clone/adapt workflows.
	// Falls back to name-only format if the prompt grows too large.
	writeFullEntities(&sb, cfg, agents)

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

// writeFullEntities appends detailed existing entity information to the prompt
// so the wizard can clone and adapt existing configurations.
func writeFullEntities(sb *strings.Builder, cfg *config.Config, agents []subagent.SubAgent) {
	if len(cfg.Tasks) > 0 {
		sb.WriteString("## Existing Tasks (use as reference or avoid name conflicts)\n\n")
		for _, t := range cfg.Tasks {
			writeTask(sb, t)
		}
		sb.WriteString("\n")
	}

	if len(cfg.Pipelines) > 0 {
		sb.WriteString("## Existing Pipelines (use as reference or avoid name conflicts)\n\n")
		for _, p := range cfg.Pipelines {
			writePipeline(sb, p)
		}
		sb.WriteString("\n")
	}

	if len(cfg.Domains) > 0 {
		sb.WriteString("## Existing Domains (use as reference or avoid name conflicts)\n\n")
		for name, d := range cfg.Domains {
			writeDomain(sb, name, d)
		}
		sb.WriteString("\n")
	}

	if len(agents) > 0 {
		sb.WriteString("## Existing Agents (use as reference or avoid name conflicts)\n\n")
		for _, a := range agents {
			writeAgent(sb, a)
		}
		sb.WriteString("\n")
	}
}

func writeTask(sb *strings.Builder, t config.Task) {
	fmt.Fprintf(sb, "- name: %s\n", t.Name)
	fmt.Fprintf(sb, "  prompt: %q\n", truncateField(t.Prompt, 500))
	if t.WorkDir != "" {
		fmt.Fprintf(sb, "  work_dir: %s\n", t.WorkDir)
	}
	if t.Schedule != "" {
		fmt.Fprintf(sb, "  schedule: %q\n", t.Schedule)
	}
	if t.Model != "" {
		fmt.Fprintf(sb, "  model: %s\n", t.Model)
	}
	if t.Timeout != "" {
		fmt.Fprintf(sb, "  timeout: %s\n", t.Timeout)
	}
	if len(t.Agents) > 0 {
		fmt.Fprintf(sb, "  agents: [%s]\n", strings.Join(t.Agents, ", "))
	}
	if len(t.MCPServers) > 0 {
		fmt.Fprintf(sb, "  mcp_servers: [%s]\n", strings.Join(t.MCPServers, ", "))
	}
	if len(t.AllowedTools) > 0 {
		fmt.Fprintf(sb, "  allowed_tools: [%s]\n", strings.Join(t.AllowedTools, ", "))
	}
	if t.Domain != "" {
		fmt.Fprintf(sb, "  domain: %s\n", t.Domain)
	}
	if t.MaxTurns > 0 {
		fmt.Fprintf(sb, "  max_turns: %d\n", t.MaxTurns)
	}
	if t.MaxBudgetUSD > 0 {
		fmt.Fprintf(sb, "  max_budget_usd: %.2f\n", t.MaxBudgetUSD)
	}
	if t.PermissionMode != "" {
		fmt.Fprintf(sb, "  permission_mode: %s\n", t.PermissionMode)
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(sb, "  tags: [%s]\n", strings.Join(t.Tags, ", "))
	}
}

func writePipeline(sb *strings.Builder, p config.Pipeline) {
	fmt.Fprintf(sb, "- name: %s\n", p.Name)
	if p.Mode != "" {
		fmt.Fprintf(sb, "  mode: %s\n", p.Mode)
	}
	if len(p.Steps) > 0 {
		steps := make([]string, len(p.Steps))
		for i, s := range p.Steps {
			steps[i] = s.Task
		}
		fmt.Fprintf(sb, "  steps: [%s]\n", strings.Join(steps, ", "))
	}
	if p.MaxIterations > 0 {
		fmt.Fprintf(sb, "  max_iterations: %d\n", p.MaxIterations)
	}
	if p.StopSignal != "" {
		fmt.Fprintf(sb, "  stop_signal: %q\n", p.StopSignal)
	}
	if p.SessionChain {
		sb.WriteString("  session_chain: true\n")
	}
	if p.Schedule != "" {
		fmt.Fprintf(sb, "  schedule: %q\n", p.Schedule)
	}
}

func writeDomain(sb *strings.Builder, name string, d config.Domain) {
	fmt.Fprintf(sb, "- name: %s\n", name)
	if d.Description != "" {
		fmt.Fprintf(sb, "  description: %q\n", d.Description)
	}
	if d.DataDir != "" {
		fmt.Fprintf(sb, "  data_dir: %s\n", d.DataDir)
	}
	if d.DB != "" {
		fmt.Fprintf(sb, "  db: %s\n", d.DB)
	}
	if d.Schema != "" {
		fmt.Fprintf(sb, "  schema: %q\n", truncateField(d.Schema, 400))
	}
	if len(d.Tasks) > 0 {
		fmt.Fprintf(sb, "  tasks: [%s]\n", strings.Join(d.Tasks, ", "))
	}
	if len(d.Pipelines) > 0 {
		fmt.Fprintf(sb, "  pipelines: [%s]\n", strings.Join(d.Pipelines, ", "))
	}
	if len(d.Agents) > 0 {
		fmt.Fprintf(sb, "  agents: [%s]\n", strings.Join(d.Agents, ", "))
	}
	if len(d.MCPServers) > 0 {
		fmt.Fprintf(sb, "  mcp_servers: [%s]\n", strings.Join(d.MCPServers, ", "))
	}
}

func writeAgent(sb *strings.Builder, a subagent.SubAgent) {
	fmt.Fprintf(sb, "- name: %s\n", a.Name)
	fmt.Fprintf(sb, "  description: %q\n", a.Description)
	if len(a.Tools) > 0 {
		fmt.Fprintf(sb, "  tools: [%s]\n", strings.Join(a.Tools, ", "))
	}
	if a.Model != "" {
		fmt.Fprintf(sb, "  model: %s\n", a.Model)
	}
	if a.PermissionMode != "" {
		fmt.Fprintf(sb, "  permission_mode: %s\n", a.PermissionMode)
	}
	if a.Instructions != "" {
		fmt.Fprintf(sb, "  instructions: %q\n", truncateField(a.Instructions, 300))
	}
	if a.Scope != "" {
		fmt.Fprintf(sb, "  scope: %s\n", a.Scope)
	}
}

// truncateField shortens a string to maxLen, appending a note about total length.
func truncateField(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... (%d chars total)", len(s))
}
