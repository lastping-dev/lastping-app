// Package mcpusage is the wire contract between the LastPing MCP servers and
// the API for attributing a REST request to the MCP tool call that made it.
// The header names and values match the hosted server at mcp.lastping.dev
// byte for byte.
//
// The MCP server sets ToolHeader (the tool's name) and CallHeader (a fresh id
// per tool call) on every REST request it sends while running a tool, so the
// API can count which tools connected assistants call. The header is a
// LABEL, never a credential: the request is authenticated and scoped exactly
// as it would be without it. Only the tool's name, a random per-call id and
// the setup-offer event travel: never an argument or a result.
package mcpusage

const (
	// ToolHeader names the tool (or, with PromptPrefix, the prompt) that a
	// REST request was made for.
	ToolHeader = "X-LastPing-MCP-Tool"
	// CallHeader carries one id per tool call, a UUID, so the several REST
	// requests one tool call may make are recorded as one call.
	CallHeader = "X-LastPing-MCP-Call"
	// PromptPrefix marks a ToolHeader value as a prompt fetch (prompts/get)
	// rather than a tool call.
	PromptPrefix = "prompt:"
	// EventHeader carries something the MCP server did with a tool call's
	// result that the API cannot see in its own response. The only value is
	// EventSetupOffer.
	EventHeader = "X-LastPing-MCP-Event"
	// EventSetupOffer: the tool's result was an empty list carrying the
	// setup offer (internal/mcptools/firstrun.go). Sent on one extra request
	// with the tool call's own call id, so it marks that call.
	EventSetupOffer = "setup_offer"

	// MonitorsHeader is a response header the API sets, to "0", on a
	// key-authenticated /api/v1 response when the project has no monitors
	// and the credential can write monitors. The MCP server reads it to know,
	// without a request of its own, that a tool result should carry the
	// setup note.
	MonitorsHeader = "X-LastPing-Monitors"
	// NudgeExcludedTool is the one tool whose successful result never carries
	// the setup note (it creates the monitor the note asks for).
	NudgeExcludedTool = "create_monitor"
)

// PromptLabel is the ToolHeader value for a fetch of prompt name.
func PromptLabel(name string) string { return PromptPrefix + name }
