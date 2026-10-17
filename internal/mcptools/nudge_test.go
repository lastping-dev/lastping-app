package mcptools_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// signalStub answers every request with status and body, setting
// X-LastPing-Monitors: 0 when empty is true, and records each request's path
// and mcpusage event.
type signalStub struct {
	mu     sync.Mutex
	paths  []string
	events []string
}

func newSignalStub(t *testing.T, empty bool, status int, body string) (*signalStub, *mcptools.APIClient) {
	t.Helper()
	h := &signalStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.paths = append(h.paths, r.URL.Path)
		h.events = append(h.events, r.Header.Get(mcpusage.EventHeader))
		h.mu.Unlock()
		if empty {
			w.Header().Set(mcpusage.MonitorsHeader, "0")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/whoami" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return h, mcptools.NewAPIClient(srv.URL, "test-key")
}

const channelBody = `{"id":"ch-1","kind":"ntfy","name":"phone","target":"https://ntfy.sh/t","verified":true,"disabled":false,"created_at":"2026-07-20T00:00:00Z"}`

// TestSetupNudge_AppendedWhenTheAPISaysNoMonitors: a successful
// create_destination on a project the API reports empty ends with the note,
// as its own text content, and the server sends no request to record it (the
// API records it from its own header). Mutation-proof: drop the append in
// addSetupNudge and this fails.
func TestSetupNudge_AppendedWhenTheAPISaysNoMonitors(t *testing.T) {
	h, c := newSignalStub(t, true, http.StatusCreated, channelBody)
	s := newTestServer(t, "https://ping.lastping.dev")
	res := callTool(t, s, c, "create_destination", map[string]interface{}{
		"kind": "ntfy", "name": "phone", "topic_url": "https://ntfy.sh/t",
	})
	require.False(t, res.IsError)
	require.True(t, mcptools.CarriesSetupNudge(res))
	require.Len(t, res.Content, 2, "the note is its own content item; the tool's own text is untouched")
	require.NotContains(t, res.Content[0].(mcp.TextContent).Text, mcptools.SetupNudgeText)
	require.Equal(t, []string{"/api/v1/channels"}, h.paths, "no note request")
	require.Equal(t, []string{""}, h.events)
}

// TestSetupNudge_AbsentWithoutTheSignal: the same call against an API that
// sends no header (a project with monitors) carries no note and sends no note
// request. The positive companion of the test above.
func TestSetupNudge_AbsentWithoutTheSignal(t *testing.T) {
	h, c := newSignalStub(t, false, http.StatusCreated, channelBody)
	s := newTestServer(t, "https://ping.lastping.dev")
	res := callTool(t, s, c, "create_destination", map[string]interface{}{
		"kind": "ntfy", "name": "phone", "topic_url": "https://ntfy.sh/t",
	})
	require.False(t, res.IsError)
	require.False(t, mcptools.CarriesSetupNudge(res))
	require.Len(t, res.Content, 1)
	require.Equal(t, []string{"/api/v1/channels"}, h.paths)
}

// TestSetupNudge_Exclusions: with the signal on every response, an errored
// call, a successful create_monitor and an empty list carrying the offer get
// no note, and only the offer is noted.
func TestSetupNudge_Exclusions(t *testing.T) {
	s := newTestServer(t, "https://ping.lastping.dev")

	_, c := newSignalStub(t, true, http.StatusBadRequest, `{"title":"Bad Request","status":400,"detail":"nope"}`)
	res := callTool(t, s, c, "create_destination", map[string]interface{}{"kind": "webhook", "name": "x"})
	require.True(t, res.IsError)
	require.False(t, mcptools.CarriesSetupNudge(res), "an errored call")

	h, c := newSignalStub(t, true, http.StatusCreated,
		`{"id":"abc-123","name":"Nightly","slug":"nightly","status":"new","schedule_kind":"simple","period_s":3600,"grace_s":300}`)
	res = callTool(t, s, c, "create_monitor", map[string]interface{}{
		"name": "Nightly", "slug": "nightly", "schedule_kind": "simple", "period_s": 3600, "grace_s": 300,
	})
	require.False(t, res.IsError, resultText(t, res))
	require.False(t, mcptools.CarriesSetupNudge(res), "a successful create_monitor")
	for _, e := range h.events {
		require.Empty(t, e, "no note request after create_monitor")
	}

	h, c = newSignalStub(t, true, http.StatusOK, `[]`)
	res = callTool(t, s, c, "list_monitors", map[string]interface{}{})
	require.True(t, mcptools.CarriesSetupOffer(res))
	require.Zero(t, strings.Count(resultText(t, res), mcptools.SetupNudgeText), "no double text")
	require.Equal(t, []string{"", mcpusage.EventSetupOffer}, h.events)
}

// TestSetupNudgeText_Shape: one short line, offer once, nothing created
// before the person agrees, no em dash.
func TestSetupNudgeText_Shape(t *testing.T) {
	txt := mcptools.SetupNudgeText
	require.LessOrEqual(t, len(txt), 300)
	require.NotContains(t, txt, "\n")
	require.NotContains(t, txt, "—")
	require.Contains(t, txt, "offer once")
	require.Contains(t, txt, "only after they agree")
}
