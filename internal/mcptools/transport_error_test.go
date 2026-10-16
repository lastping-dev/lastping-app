package mcptools_test

import (
	"bytes"
	"log"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
)

// A tool call whose API request gets no HTTP response used to return the
// transport error's text as tool output: client.Do's "Get \"<url>\": dial
// tcp <address>: ...", naming the API's URL and the address it dialled.
// The output must be a fixed sentence now, and the log must keep the cause
// without the request URL (whose path and query carry the caller's
// arguments).
func TestTool_APIUnreachableShowsFixedText(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	base := "http://" + ln.Addr().String() + "/SECRETPATH"
	require.NoError(t, ln.Close())

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	c := mcptools.NewAPIClient(base, "test-key")
	s := newTestServer(t, "https://ping.lastping.dev")
	result := callTool(t, s, c, "list_deliveries", map[string]interface{}{"monitor": "SECRET123"})
	log.SetOutput(prev)

	require.True(t, result.IsError)
	require.Equal(t, "API request failed: the LastPing API could not be reached; try again in a minute", extractText(result))

	out := buf.String()
	require.Contains(t, out, "connection refused", "the log lost the cause")
	for _, leak := range []string{"SECRET123", "SECRETPATH"} {
		require.False(t, strings.Contains(out, leak), "the log carries %q: %s", leak, out)
	}
}

// A request that cannot be built fails in url.Parse, whose text quotes the
// whole URL ("parse \"<url>\": net/url: invalid control character in URL").
// The output must be the fixed sentence, and the log keeps the cause without
// the URL.
func TestTool_RequestBuildFailureShowsFixedText(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	c := mcptools.NewAPIClient("http://127.0.0.1:1/SECRETPATH\x7f", "test-key")
	s := newTestServer(t, "https://ping.lastping.dev")
	tools := map[string]map[string]interface{}{
		"list_monitors":       {},
		"list_deliveries":     {},
		"get_alert_templates": {"id": "00000000-0000-0000-0000-000000000001"},
	}
	for tool, args := range tools {
		result := callTool(t, s, c, tool, args)
		require.True(t, result.IsError, tool)
		require.Equal(t, "could not build the LastPing API request; check that LASTPING_BASE_URL is a valid URL "+
			"and that no argument contains a control character", extractText(result), tool)
	}
	log.SetOutput(prev)

	out := buf.String()
	require.Contains(t, out, "invalid control character", "the log lost the cause")
	require.NotContains(t, out, "SECRETPATH", "the log carries the URL")
}
