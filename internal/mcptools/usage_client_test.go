package mcptools_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
	"github.com/lastping-dev/lastping-app/internal/mcpusage"
)

// stdioSession drives the real stdio transport, the one the lastping-mcp
// binary serves, over a pipe.
type stdioSession struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
}

// startStdio serves a fresh server over stdio with c in every request's
// context, exactly as cmd/lastping-mcp does, and stops it when t ends.
func startStdio(t *testing.T, c *mcptools.APIClient) *stdioSession {
	t.Helper()
	s := mcptools.NewServer("test", "https://ping.lastping.dev")
	stdio := server.NewStdioServer(s)
	stdio.SetContextFunc(func(ctx context.Context) context.Context {
		return mcptools.ContextWithClient(ctx, c)
	})
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = stdio.Listen(ctx, inR, outW)
	}()
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		_ = outR.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("stdio server did not stop")
		}
	})
	return &stdioSession{t: t, in: inW, out: bufio.NewReader(outR)}
}

// call sends one JSON-RPC request and returns its response line.
func (ss *stdioSession) call(id int, method string, params map[string]any) map[string]any {
	ss.t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	require.NoError(ss.t, err)
	_, err = ss.in.Write(append(raw, '\n'))
	require.NoError(ss.t, err)
	for {
		line, err := ss.out.ReadBytes('\n')
		require.NoError(ss.t, err)
		var msg map[string]any
		require.NoError(ss.t, json.Unmarshal(line, &msg))
		if msg["id"] != nil { // skip notifications
			require.Nil(ss.t, msg["error"], "%s failed: %s", method, line)
			return msg
		}
	}
}

func (ss *stdioSession) initialize(clientName string) {
	ss.t.Helper()
	ss.call(1, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "1.2.3"},
	})
}

// recordClients returns the stub's recorded ClientHeader and ToolHeader
// values and a client pointed at it.
func recordClients(t *testing.T, body string) (*[]string, *[]string, *mcptools.APIClient) {
	t.Helper()
	h, c := newHeaderStub(t, body)
	return &h.clients, &h.tools, c
}

// TestClientCategory_KnownClientLabelsToolRequests: a session that
// initialized as Claude Code sends the claude-code category on every REST
// request its tool calls make, the note included, and never the raw name.
func TestClientCategory_KnownClientLabelsToolRequests(t *testing.T) {
	clients, tools, c := recordClients(t, `[]`)
	ss := startStdio(t, c)
	ss.initialize("claude-code")

	ss.call(2, "tools/call", map[string]any{"name": "list_monitors", "arguments": map[string]any{}})
	ss.call(3, "tools/call", map[string]any{"name": "list_destinations", "arguments": map[string]any{}})

	require.Equal(t, []string{"list_monitors", "list_monitors", "list_destinations", "list_destinations"}, *tools,
		"each empty list makes its request and the setup-offer note")
	require.Equal(t, []string{"claude-code", "claude-code", "claude-code", "claude-code"}, *clients)
}

// TestClientCategory_CodexIsMapped: the second confirmed name maps to its
// category, so the table is read rather than the name echoed.
func TestClientCategory_CodexIsMapped(t *testing.T) {
	clients, _, c := recordClients(t, `[{"id":"abc-123","name":"Nightly backup","slug":"nightly-backup","status":"up","schedule_kind":"simple","period_s":3600,"grace_s":300}]`)
	ss := startStdio(t, c)
	ss.initialize("codex-mcp-client")

	ss.call(2, "tools/call", map[string]any{"name": "list_monitors", "arguments": map[string]any{}})

	require.Equal(t, []string{"codex"}, *clients)
}

// TestClientCategory_UnknownClientSendsNoHeader: an unrecognised name sends
// no category at all. Its positive companion is
// TestClientCategory_KnownClientLabelsToolRequests; the tool label still
// travels, so the request was labelled and only the category is absent.
func TestClientCategory_UnknownClientSendsNoHeader(t *testing.T) {
	for _, name := range []string{"my-own-agent", "CLAUDE-CODE", "claude-code/2.1.302", ""} {
		t.Run(name, func(t *testing.T) {
			clients, tools, c := recordClients(t, `[{"id":"abc-123","name":"Nightly backup","slug":"nightly-backup","status":"up","schedule_kind":"simple","period_s":3600,"grace_s":300}]`)
			ss := startStdio(t, c)
			ss.initialize(name)

			ss.call(2, "tools/call", map[string]any{"name": "list_monitors", "arguments": map[string]any{}})

			require.Equal(t, []string{"list_monitors"}, *tools)
			require.Equal(t, []string{""}, *clients)
		})
	}
}

// TestClientWireNames_MatchTheHostedServer pins the header name and the
// categories to the literal strings the hosted server at mcp.lastping.dev
// sends. The tests above use the constants, so a changed constant would pass
// them while the API dropped every category.
func TestClientWireNames_MatchTheHostedServer(t *testing.T) {
	require.Equal(t, "X-LastPing-MCP-Client", mcpusage.ClientHeader)
	require.Equal(t, "claude", mcpusage.ClientClaude)
	require.Equal(t, "claude-code", mcpusage.ClientClaudeCode)
	require.Equal(t, "chatgpt", mcpusage.ClientChatGPT)
	require.Equal(t, "codex", mcpusage.ClientCodex)
	require.Equal(t, "cursor", mcpusage.ClientCursor)
	require.Equal(t, "vscode", mcpusage.ClientVSCode)
}

// TestClientFromName_Table pins every mapped clientInfo name, and that a
// near miss maps to nothing.
func TestClientFromName_Table(t *testing.T) {
	for name, want := range map[string]string{
		"claude-code":                   "claude-code",
		"codex-mcp-client":              "codex",
		"cursor-vscode":                 "cursor",
		"Cursor":                        "cursor",
		"Visual Studio Code":            "vscode",
		"Visual Studio Code - Insiders": "vscode",
		"claude-desktop-3p":             "claude",
		"claude-ai":                     "",
		"openai-mcp":                    "",
		"cursor":                        "",
		"Claude Code":                   "",
		"":                              "",
	} {
		require.Equal(t, want, mcpusage.ClientFromName(name), "clientInfo.name %q", name)
	}
}
