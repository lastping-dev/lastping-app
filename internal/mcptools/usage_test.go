package mcptools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// headerStub records the mcpusage headers of every request it receives and
// answers each with body.
type headerStub struct {
	mu     sync.Mutex
	tools  []string
	calls  []string
	paths  []string
	events []string
	// clients holds each request's mcpusage ClientHeader.
	clients []string
}

func newHeaderStub(t *testing.T, body string) (*headerStub, *mcptools.APIClient) {
	t.Helper()
	h := &headerStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.tools = append(h.tools, r.Header.Get(mcpusage.ToolHeader))
		h.calls = append(h.calls, r.Header.Get(mcpusage.CallHeader))
		h.paths = append(h.paths, r.URL.Path)
		h.events = append(h.events, r.Header.Get(mcpusage.EventHeader))
		h.clients = append(h.clients, r.Header.Get(mcpusage.ClientHeader))
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return h, mcptools.NewAPIClient(srv.URL, "test-key")
}

// TestToolCall_LabelsEveryRESTRequest: a tool call through the real server
// sends the tool's name and a UUID call id, and a second call gets a new id.
func TestToolCall_LabelsEveryRESTRequest(t *testing.T) {
	h, c := newHeaderStub(t, `[{"id":"abc-123","name":"Nightly backup","slug":"nightly-backup","status":"up","schedule_kind":"simple","period_s":3600,"grace_s":300}]`)
	s := newTestServer(t, "https://ping.lastping.dev")

	callTool(t, s, c, "list_monitors", map[string]interface{}{})
	callTool(t, s, c, "list_monitors", map[string]interface{}{})

	require.Equal(t, []string{"list_monitors", "list_monitors"}, h.tools)
	require.Len(t, h.calls, 2)
	for _, id := range h.calls {
		_, err := uuid.Parse(id)
		require.NoError(t, err, "call id %q must be a UUID", id)
	}
	require.NotEqual(t, h.calls[0], h.calls[1], "each tool call has its own id")
	require.Equal(t, []string{"", ""}, h.events, "a non-empty list serves no offer")
}

// TestToolCall_EmptyListNotesTheSetupOffer: an empty list's result carries the
// setup offer, and the server tells the API so with one more request under the
// same call id. Its positive-absence companion is the last line of
// TestToolCall_LabelsEveryRESTRequest.
func TestToolCall_EmptyListNotesTheSetupOffer(t *testing.T) {
	h, c := newHeaderStub(t, `[]`)
	s := newTestServer(t, "https://ping.lastping.dev")
	res := callTool(t, s, c, "list_monitors", map[string]interface{}{})
	require.True(t, mcptools.CarriesSetupOffer(res))

	require.Equal(t, []string{"/api/v1/checks", "/api/v1/whoami"}, h.paths)
	require.Equal(t, []string{"list_monitors", "list_monitors"}, h.tools)
	require.Equal(t, h.calls[0], h.calls[1], "the note marks the same call")
	require.Equal(t, []string{"", "setup_offer"}, h.events)
}

// TestPromptFetch_IsNoted: prompts/get makes one labelled whoami request, and
// returns the prompt whether or not the API answered.
func TestPromptFetch_IsNoted(t *testing.T) {
	h, c := newHeaderStub(t, `{}`)
	s := mcptools.NewServer("test", "https://ping.lastping.dev")
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "prompts/get",
		"params": map[string]any{"name": "set_up_monitoring"}})
	require.NoError(t, err)
	resp := s.HandleMessage(mcptools.ContextWithClient(context.Background(), c), raw)
	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "got %T", resp)
	_, ok = jr.Result.(mcp.GetPromptResult)
	require.True(t, ok, "got %T", jr.Result)

	require.Equal(t, []string{"prompt:set_up_monitoring"}, h.tools)
	require.Equal(t, []string{"/api/v1/whoami"}, h.paths)
}

// TestWireNames_MatchTheHostedServer pins the header names and values to the
// literal strings the hosted server at mcp.lastping.dev sends. The tests
// above use the constants, so a changed constant would pass them while the
// API dropped every label.
func TestWireNames_MatchTheHostedServer(t *testing.T) {
	require.Equal(t, "X-LastPing-MCP-Tool", mcpusage.ToolHeader)
	require.Equal(t, "X-LastPing-MCP-Call", mcpusage.CallHeader)
	require.Equal(t, "X-LastPing-MCP-Event", mcpusage.EventHeader)
	require.Equal(t, "setup_offer", mcpusage.EventSetupOffer)
	require.Equal(t, "prompt:set_up_monitoring", mcpusage.PromptLabel(mcptools.SetUpMonitoringPrompt))
	require.Equal(t, "X-LastPing-Monitors", mcpusage.MonitorsHeader)
	require.Equal(t, "create_monitor", mcpusage.NudgeExcludedTool)
}
