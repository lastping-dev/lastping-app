package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
