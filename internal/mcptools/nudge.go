package mcptools

// nudge.go: the one-line setup note on every tool result while the project
// has no monitors. It matches the hosted server at mcp.lastping.dev.
//
// The empty-list offer (firstrun.go) only appears when an assistant lists
// monitors, agents or destinations and the list is empty. An assistant that
// sets up a destination and never lists monitors never sees it. So while the
// project has no monitors, any other result says so in one line, with the
// same rule the offer keeps: offer once, and nothing is created until the
// person agrees.
//
// THE SIGNAL. The API sets X-LastPing-Monitors: 0 on a key-authenticated
// response when the project has no monitors. do reads it into the tool
// call's context (readMonitorsSignal); the latest response of the call wins.
// No extra request is made to learn it.
//
// WHEN THE NOTE IS LEFT OUT. A call that errored (a Go error or IsError), a
// result already carrying the empty-list offer (no double text), a
// successful create_monitor (the project now has one), and any call whose API
// responses carried no "0".
//
// No request is made to record that the note was served: the API already
// knows it sent the header.

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// SetupNudgeText is the line appended to a tool result while the project has
// no monitors.
const SetupNudgeText = "Note: this project has no monitors yet, so nothing here will alert. " +
	"If the user hasn't asked for something else, offer once to set up monitoring for what they run unattended " +
	"(AI agent sessions, scheduled tasks, cron jobs, CI); " +
	"only after they agree, call create_monitor then get_ping_instructions."

// callSignal is what the API's responses to one tool call said.
type callSignal struct {
	noMonitors atomic.Bool
}

// readMonitorsSignal records resp's X-LastPing-Monitors answer on the tool
// call req belongs to. The latest response wins, so a call that creates the
// first monitor and reads it back ends up with no signal.
func readMonitorsSignal(req *http.Request, resp *http.Response) {
	tc, ok := callFromContext(req.Context())
	if !ok || tc.signal == nil {
		return
	}
	tc.signal.noMonitors.Store(resp.Header.Get(mcpusage.MonitorsHeader) == "0")
}

// addSetupNudge appends SetupNudgeText to res as its own text content when
// the call's API responses said the project has no monitors and none of the
// exclusions above applies.
func addSetupNudge(ctx context.Context, tool string, res *mcp.CallToolResult) {
	if res == nil || res.IsError || tool == mcpusage.NudgeExcludedTool {
		return
	}
	tc, ok := callFromContext(ctx)
	if !ok || tc.signal == nil || !tc.signal.noMonitors.Load() {
		return
	}
	res.Content = append(res.Content, mcp.NewTextContent(SetupNudgeText))
}

// CarriesSetupNudge reports whether a tool result ends with the setup note.
func CarriesSetupNudge(res *mcp.CallToolResult) bool {
	if res == nil || len(res.Content) == 0 {
		return false
	}
	tc, ok := res.Content[len(res.Content)-1].(mcp.TextContent)
	return ok && tc.Text == SetupNudgeText
}
