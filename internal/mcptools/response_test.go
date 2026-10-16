package mcptools_test

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
)

// resetMidBody starts a raw TCP server that answers every request with the
// status line and headers, a partial body (Content-Length promises far more),
// and then a TCP reset. The client's body read fails with "read tcp
// <local>-><remote>: connection reset by peer", text that must never reach
// tool output verbatim.
func resetMidBody(t *testing.T, status string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			r := bufio.NewReader(c)
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == "\r\n" {
					break
				}
			}
			_, _ = c.Write([]byte("HTTP/1.1 " + status + "\r\nContent-Type: application/json\r\nContent-Length: 5000\r\n\r\n{\"deliveries\":[{\"id\":"))
			// Give the client time to read the headers and the partial body
			// before the reset, so the failure is a body read, not Do's.
			time.Sleep(50 * time.Millisecond)
			_ = c.(*net.TCPConn).SetLinger(0)
			_ = c.Close()
		}
	}()
	return "http://" + ln.Addr().String()
}

var (
	ipLiteral = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	// leakWords are fragments of transport and decoder errors; none may reach
	// tool output.
	leakWords = []string{"tcp", "reset", "EOF", "127.0.0.1", "http://", "invalid character", "unexpected end"}
)

func requireFixedText(t *testing.T, tool, got, want string) {
	t.Helper()
	require.Equal(t, want, got, "%s: tool output", tool)
	require.False(t, ipLiteral.MatchString(got), "%s: output carries an IP: %q", tool, got)
	for _, w := range leakWords {
		require.NotContains(t, got, w, "%s: output carries %q", tool, w)
	}
}

// A reset after the headers and part of the body, on a 200: every tool shows
// the same fixed sentence and the log keeps the cause.
func TestTool_BodyResetShowsFixedText(t *testing.T) {
	base := resetMidBody(t, "200 OK")

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	c := mcptools.NewAPIClient(base, "test-key")
	s := newTestServer(t, "https://ping.lastping.dev")
	tools := map[string]map[string]interface{}{
		"list_deliveries":   {},
		"list_monitors":     {},
		"get_monitor":       {"id": "00000000-0000-0000-0000-000000000001"},
		"list_destinations": {},
		"list_agents":       {},
		"list_api_keys":     {},
		"export_terraform":  {},
		"list_status_pages": {},
	}
	for tool, args := range tools {
		res := callTool(t, s, c, tool, args)
		require.True(t, res.IsError, "%s: not an error result", tool)
		requireFixedText(t, tool, extractText(res), "the LastPing API response was interrupted; try again in a minute")
	}
	log.SetOutput(prev)
	require.Contains(t, buf.String(), "reading a LastPing API response", "the log lost the cause")
}

// The same reset on an error status, where the body is read as a problem
// document.
func TestTool_ProblemBodyResetShowsFixedText(t *testing.T) {
	base := resetMidBody(t, "500 Internal Server Error")
	c := mcptools.NewAPIClient(base, "test-key")
	s := newTestServer(t, "https://ping.lastping.dev")
	for _, tool := range []string{"list_deliveries", "list_monitors"} {
		res := callTool(t, s, c, tool, map[string]interface{}{})
		require.True(t, res.IsError)
		requireFixedText(t, tool, extractText(res), "the LastPing API response was interrupted; try again in a minute")
	}
}

// A complete 200 body that is not the JSON the tool expects is a contract
// mismatch: a fixed sentence, not the decoder's text.
func TestTool_UndecodableBodyShowsFixedText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()
	c := mcptools.NewAPIClient(srv.URL, "test-key")
	s := newTestServer(t, "https://ping.lastping.dev")
	for _, tool := range []string{"list_deliveries", "list_monitors", "list_agents"} {
		res := callTool(t, s, c, tool, map[string]interface{}{})
		require.True(t, res.IsError)
		requireFixedText(t, tool, extractText(res), "unexpected response from the LastPing API")
	}
}

// A client timeout is not "could not be reached": the API may have the
// request. Both the wait for headers and the wait for the body are covered.
func TestTool_TimeoutShowsDidNotRespond(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/checks" { // headers sent, body withheld
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("["))
			w.(http.Flusher).Flush()
		}
		<-release
	}))
	defer srv.Close()
	defer close(release)

	c := mcptools.NewAPIClient(srv.URL, "test-key")
	c.HTTP.Timeout = 300 * time.Millisecond
	s := newTestServer(t, "https://ping.lastping.dev")

	res := callTool(t, s, c, "list_deliveries", map[string]interface{}{})
	require.True(t, res.IsError)
	requireFixedText(t, "list_deliveries", extractText(res), "API request failed: the LastPing API did not respond in time; try again")

	res = callTool(t, s, c, "list_monitors", map[string]interface{}{})
	require.True(t, res.IsError)
	requireFixedText(t, "list_monitors", extractText(res), "the LastPing API did not respond in time; try again")
}

// Every request and every response-body read goes through response.go, the
// one file that turns transport and read errors into fixed text. A new tool
// that calls c.HTTP.Do or reads resp.Body itself would show the transport's
// text (request URL, internal addresses) to the customer again.
func TestResponseHandling_OnlyInResponseFile(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	forbidden := regexp.MustCompile(`HTTP\.Do\(|io\.ReadAll\(|json\.NewDecoder\(|resp\.Body\b(?:[^.]|\.[^C])`)
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "response.go" {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		checked++
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			require.False(t, forbidden.MatchString(line),
				"%s:%d sends a request or reads a response body outside response.go: %s", f, i+1, strings.TrimSpace(line))
		}
	}
	require.Greater(t, checked, 10, "the scan found too few files to mean anything")
}
