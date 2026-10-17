package mcpusage

// client.go: which assistant a labelled REST request was made for.
//
// This server runs over stdio, one process per assistant session, so the
// initialize message's clientInfo is still known when a tool runs. The server
// maps clientInfo.name to a category with the fixed table below and sends the
// category, never the name, in ClientHeader on each labelled REST request.
// The header name and categories match the hosted server at mcp.lastping.dev
// byte for byte.
//
// The header is a LABEL, like ToolHeader: any key holder can send any
// allowlisted value. The API accepts only the categories below and stores
// nothing else. Read the result as indicative.

// ClientHeader carries the category of the MCP client a labelled request
// was made for, one of the categories below. Absent when the client was not
// recognised.
const ClientHeader = "X-LastPing-MCP-Client"

// The categories ClientHeader may carry. An unrecognised client sends no
// header and is recorded as "client unknown".
const (
	ClientClaude     = "claude"
	ClientClaudeCode = "claude-code"
	ClientChatGPT    = "chatgpt"
	ClientCodex      = "codex"
	ClientCursor     = "cursor"
	ClientVSCode     = "vscode"
)

// clientInfoNames maps an initialize request's clientInfo.name to a category.
// Exact, case-sensitive matches only: a name that is not listed here sends
// no header. Evidence, 2026-10-17:
//
//	claude-code                    Claude Code, confirmed on the wire
//	codex-mcp-client               Codex CLI, confirmed in its source
//	                               (codex-rs/codex-mcp/src/rmcp_client.rs)
//	cursor-vscode                  Cursor, confirmed in its shipped source
//	                               (its MCP process and cursor-mcp extension)
//	Cursor                         Cursor, confirmed in its shipped source:
//	                               the workbench MCP handler it inherits from
//	                               VS Code sends the product's nameLong, which
//	                               is "Cursor"
//	Visual Studio Code             VS Code (Copilot), confirmed in its source:
//	Visual Studio Code - Insiders  mcpServerRequestHandler.ts sends
//	                               productService.nameLong, the release
//	                               builds' product names
//	claude-desktop-3p              Claude Desktop in a third-party cloud
//	                               deployment, confirmed in its shipped source
//
// Deliberately not mapped, because only other servers' observations support
// them (inferred, never confirmed from the client itself): "claude-ai" for
// Claude Desktop's own local servers, and "openai-mcp" for ChatGPT, which
// connects to remote servers only and never launches a stdio binary.
var clientInfoNames = map[string]string{
	"claude-code":                   ClientClaudeCode,
	"codex-mcp-client":              ClientCodex,
	"cursor-vscode":                 ClientCursor,
	"Cursor":                        ClientCursor,
	"Visual Studio Code":            ClientVSCode,
	"Visual Studio Code - Insiders": ClientVSCode,
	"claude-desktop-3p":             ClientClaude,
}

// ClientFromName returns the category of the MCP client whose initialize
// request carried clientInfo.name name, or an empty string when it is not one
// of the recognised clients. The name itself is never returned.
func ClientFromName(name string) string {
	return clientInfoNames[name]
}
