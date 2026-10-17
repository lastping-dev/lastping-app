package mcptools

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// usage.go labels each REST request with the MCP tool call that made it, so
// the API can count which tools connected assistants actually call. Only the
// tool's NAME, a random per-call id, the category of the assistant (never its
// self-reported name) and, for an empty list, the fact that the setup offer
// was served travel: never an argument, a result or anything else the agent
// sent. It matches the hosted server at mcp.lastping.dev.

type toolCallKey struct{}

// toolCall is what a running tool's context carries.
type toolCall struct {
	label string // mcpusage ToolHeader value
	id    string // one UUID per tool call
	// signal is what the API's responses to this call said (nudge.go).
	signal *callSignal
}

// withToolCall returns ctx labelled for one call of label.
func withToolCall(ctx context.Context, label string) context.Context {
	return context.WithValue(ctx, toolCallKey{}, toolCall{label: label, id: uuid.NewString(), signal: &callSignal{}})
}

// callFromContext returns the tool call ctx belongs to, if any.
func callFromContext(ctx context.Context) (toolCall, bool) {
	tc, ok := ctx.Value(toolCallKey{}).(toolCall)
	return tc, ok
}

// labelToolCall sets the mcpusage headers on req when its context belongs to
// a tool call. Called by do, the one function every REST request goes
// through. A request made outside a tool call is left unlabelled.
func labelToolCall(req *http.Request) {
	tc, ok := callFromContext(req.Context())
	if !ok {
		return
	}
	req.Header.Set(mcpusage.ToolHeader, tc.label)
	req.Header.Set(mcpusage.CallHeader, tc.id)
	if category := clientCategory(req.Context()); category != "" {
		req.Header.Set(mcpusage.ClientHeader, category)
	}
}

// clientCategory returns the mcpusage ClientHeader category of the MCP client
// whose session ctx belongs to, or an empty string when the client is unknown
// or the session never sent initialize. The stdio transport has one session
// per process, so the category holds for the whole process. A stateless HTTP
// request carries no initialize and so no category. Only the category leaves
// this function: the clientInfo name is never logged or sent.
func clientCategory(ctx context.Context) string {
	session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo)
	if !ok {
		return ""
	}
	return mcpusage.ClientFromName(session.GetClientInfo().Name)
}

// toolCallMiddleware labels the context of every tool call with the tool's
// name before its handler runs. Register installs it, so every server built
// from this package (stdio, HTTP, tests) labels the same way.
//
// When the result carries the setup offer it also sends one cheap labelled
// request under the same call id with the setup_offer event, so the API marks
// that call. The offer is this server's reading of the API's response, so the
// API cannot see it on its own; the extra request happens only for an empty
// list.
//
// Otherwise, when the API said the project has no monitors, it appends the
// setup note (nudge.go). That sends nothing: the API records it from the
// header it sent.
func toolCallMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx = withToolCall(ctx, req.Params.Name)
		res, err := next(ctx, req)
		if err != nil {
			return res, err
		}
		if CarriesSetupOffer(res) {
			noteToAPI(ctx, mcpusage.EventSetupOffer)
		} else {
			addSetupNudge(ctx, req.Params.Name, res)
		}
		return res, err
	}
}

// setupOfferTexts are the empty-list results that offer to set things up.
var setupOfferTexts = []string{EmptyMonitorsText, EmptyAgentsText, EmptyDestinationsText}

// CarriesSetupOffer reports whether a tool result is one of the empty-list
// results carrying the setup offer (firstrun.go).
func CarriesSetupOffer(res *mcp.CallToolResult) bool {
	if res == nil || res.IsError {
		return false
	}
	for _, c := range res.Content {
		tc, ok := c.(mcp.TextContent)
		if !ok {
			continue
		}
		for _, offer := range setupOfferTexts {
			if strings.Contains(tc.Text, offer) {
				return true
			}
		}
	}
	return false
}

// noteTimeout bounds a note to the API so a slow API never holds a prompt or
// a tool result back for long.
const noteTimeout = 2 * time.Second

// notePromptFetch tells the API, best-effort, that prompt name was fetched.
// A prompt makes no REST call of its own, so without this a prompts/get left
// no trace.
func notePromptFetch(ctx context.Context, name string) {
	noteToAPI(withToolCall(ctx, mcpusage.PromptLabel(name)), "")
}

// noteToAPI sends one GET of /api/v1/whoami (the cheapest authenticated read)
// carrying ctx's tool-call label and, when event is not empty, the mcpusage
// event header. Its result and any failure are ignored. No client in ctx
// sends nothing.
//
// A note is an ordinary authenticated request: it counts against the key's
// rate limit, it is synchronous (up to noteTimeout), and it stamps the key's
// last use. That is why there are only two notes: after an empty list that
// carried the setup offer, and per prompt fetch. The setup note (nudge.go)
// sends none.
func noteToAPI(ctx context.Context, event string) {
	c, err := clientFromContext(ctx)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, noteTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/whoami", nil)
	if err != nil {
		return
	}
	c.auth(req)
	if event != "" {
		req.Header.Set(mcpusage.EventHeader, event)
	}
	resp, err := c.do(req)
	if err != nil {
		return
	}
	_, _ = readBody(resp, 4096)
	_ = resp.Body.Close()
}
