package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// TestToolCall_SharesOneIDAcrossItsRequests: the REST requests one tool call
// makes (a retry, a follow-up read) carry the same call id, so the API counts
// one call, and a request outside any tool call carries no label at all.
func TestToolCall_SharesOneIDAcrossItsRequests(t *testing.T) {
	var ids, tools []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, r.Header.Get(mcpusage.CallHeader))
		tools = append(tools, r.Header.Get(mcpusage.ToolHeader))
	}))
	t.Cleanup(srv.Close)
	c := NewAPIClient(srv.URL, "k")

	send := func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/whoami", nil)
		require.NoError(t, err)
		resp, err := c.do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	ctx := withToolCall(context.Background(), "create_api_key")
	send(ctx)
	send(ctx)
	send(context.Background())

	require.Equal(t, []string{"create_api_key", "create_api_key", ""}, tools)
	require.NotEmpty(t, ids[0])
	require.Equal(t, ids[0], ids[1])
	require.Empty(t, ids[2])
}

// clientInfoSession is a session that initialized with clientInfo info.
type clientInfoSession struct {
	info mcp.Implementation
}

func (s *clientInfoSession) Initialize()                                         {}
func (s *clientInfoSession) Initialized() bool                                   { return true }
func (s *clientInfoSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (s *clientInfoSession) SessionID() string                                   { return "test-session" }
func (s *clientInfoSession) GetClientInfo() mcp.Implementation                   { return s.info }
func (s *clientInfoSession) SetClientInfo(info mcp.Implementation)               { s.info = info }
func (s *clientInfoSession) GetClientCapabilities() mcp.ClientCapabilities {
	return mcp.ClientCapabilities{}
}
func (s *clientInfoSession) SetClientCapabilities(mcp.ClientCapabilities) {}

// TestClientCategory_OnlyOnLabelledRequests: in a session that initialized as
// Claude Code, a request made inside a tool call carries the category and one
// made outside any tool call carries nothing, the category included.
func TestClientCategory_OnlyOnLabelledRequests(t *testing.T) {
	var clients []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clients = append(clients, r.Header.Get(mcpusage.ClientHeader))
	}))
	t.Cleanup(srv.Close)
	c := NewAPIClient(srv.URL, "k")
	s := server.NewMCPServer("test", "0.0.1")
	sessionCtx := s.WithContext(context.Background(), &clientInfoSession{info: mcp.Implementation{Name: "claude-code"}})

	send := func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/whoami", nil)
		require.NoError(t, err)
		resp, err := c.do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	send(withToolCall(sessionCtx, "list_monitors"))
	send(sessionCtx)

	require.Equal(t, []string{"claude-code", ""}, clients)
}
