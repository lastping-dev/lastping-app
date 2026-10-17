package mcptools

import "github.com/mark3labs/mcp-go/server"

// Register adds all LastPing MCP tools to s.
// pingHost is the base URL for ping endpoints (e.g. "https://ping.lastping.dev").
// Each tool handler resolves its *APIClient from the request context; callers must
// seed it via ContextWithClient before the server starts handling requests.
//
// It also installs toolCallMiddleware, which labels every REST request a tool
// makes with the tool's name and appends the setup note while the project has
// no monitors (usage.go, nudge.go).
func Register(s *server.MCPServer, pingHost string) {
	s.Use(toolCallMiddleware)
	registerCheckTools(s)
	registerIncidentTools(s)
	registerChannelTools(s)
	registerRouteTools(s)
	registerStatusPageTools(s)
	registerPingTools(s, pingHost)
	registerTemplateTools(s)
	registerAPIKeyTools(s)
	registerExportTools(s)
	registerAgentTools(s)
	registerRunExpectationTools(s)
	registerRunTools(s)
	registerFailureInboxTools(s)
	registerDiscoveryTools(s)
	registerDeliveryTools(s)
	registerTraceSetupTools(s)
	registerObservabilityTools(s)
	registerPrompts(s)
}
