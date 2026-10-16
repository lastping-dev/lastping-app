package mcptools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/lastping-dev/lastping-app/internal/mcptools"
)

// firstrun_test.go: an empty project's list results carry the setup offer
// through the real server (tools/call), a non-empty one does not, and the
// set_up_monitoring prompt is listed and returns the guided flow.
//
// The phrases are literals, not the exported constants: a test comparing the
// result with the constant it was built from would pass whatever the constant
// said.

// listStub answers every GET with body.
func listStub(t *testing.T, body string) *mcptools.APIClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return mcptools.NewAPIClient(srv.URL, "test-key")
}

func TestEmptyLists_CarryTheSetupOffer(t *testing.T) {
	s := mcptools.NewServer("test", "https://ping.lastping.dev")
	cases := map[string][]string{
		"list_monitors": {
			"This project has no monitors yet.",
			"offer once to set up monitoring for what they run unattended",
			"AI agent sessions (for example Claude Code)",
			"no-code automations (n8n, Zapier, Make)",
			"cron jobs and CI workflows",
			"Propose one monitor for each.",
			"Only after they agree, call create_monitor, then get_ping_instructions",
		},
		"list_agents": {
			"This project has no agents yet.",
			"offer once to set up monitoring for the AI agents they run",
			"Only after they agree, call create_monitor",
			"register_agent",
			"get_ping_instructions",
		},
		"list_destinations": {
			"This project has no alert destinations, so alerts from its monitors are not delivered anywhere.",
			"offer once to add one where they want to be alerted",
			"Only after they choose, call create_destination, then set_route",
		},
	}
	for tool, phrases := range cases {
		t.Run(tool, func(t *testing.T) {
			res := callTool(t, s, listStub(t, `[]`), tool, map[string]interface{}{})
			require.False(t, res.IsError, tool)
			text := extractText(res)
			for _, p := range phrases {
				require.Contains(t, text, p, tool)
			}
			require.NotContains(t, text, "—", "no em dash in customer text")
		})
	}
}

// Positive companion: a project with something in it gets the data and no offer.
func TestNonEmptyLists_CarryNoSetupOffer(t *testing.T) {
	s := mcptools.NewServer("test", "https://ping.lastping.dev")
	cases := map[string]struct{ body, want string }{
		"list_monitors": {
			`[{"id":"abc-123","name":"Nightly backup","slug":"nightly-backup","status":"up","schedule_kind":"simple","period_s":3600,"grace_s":300}]`,
			"Nightly backup",
		},
		"list_agents": {
			`[{"id":"ag-1","slug":"deploy-bot","name":"Deploy Bot","status":"up","monitor_count":1}]`,
			"deploy-bot",
		},
		"list_destinations": {
			`[{"id":"ch-1","kind":"email","name":"Email"}]`,
			"ch-1",
		},
	}
	for tool, tc := range cases {
		t.Run(tool, func(t *testing.T) {
			res := callTool(t, s, listStub(t, tc.body), tool, map[string]interface{}{})
			require.False(t, res.IsError, tool)
			text := extractText(res)
			require.Contains(t, text, tc.want, tool)
			require.NotContains(t, text, "offer once", tool)
			require.NotContains(t, text, "This project has no", tool)
		})
	}
}

func rpc(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	out, err := json.Marshal(rpcResult(t, method, params))
	require.NoError(t, err)
	return out
}

func rpcResult(t *testing.T, method string, params any) any {
	t.Helper()
	s := mcptools.NewServer("test", "https://ping.lastping.dev")
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)
	resp := s.HandleMessage(context.Background(), raw)
	jr, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "expected JSONRPCResponse, got %T: %+v", resp, resp)
	return jr.Result
}

func TestPrompts_ListIncludesSetUpMonitoring(t *testing.T) {
	var res mcp.ListPromptsResult
	require.NoError(t, json.Unmarshal(rpc(t, "prompts/list", map[string]any{}), &res))
	require.Len(t, res.Prompts, 1)
	p := res.Prompts[0]
	require.Equal(t, "set_up_monitoring", p.Name)
	require.Equal(t, "Set up LastPing monitoring", p.Title)
	require.NotEmpty(t, p.Description)
	require.Len(t, p.Arguments, 1)
	require.Equal(t, "what", p.Arguments[0].Name)
	require.False(t, p.Arguments[0].Required)
}

func promptText(t *testing.T, args map[string]string) string {
	t.Helper()
	params := map[string]any{"name": "set_up_monitoring"}
	if args != nil {
		params["arguments"] = args
	}
	res, ok := rpcResult(t, "prompts/get", params).(mcp.GetPromptResult)
	require.True(t, ok, "prompts/get result type")
	require.Len(t, res.Messages, 1)
	require.Equal(t, mcp.RoleUser, res.Messages[0].Role)
	tc, ok := res.Messages[0].Content.(mcp.TextContent)
	require.True(t, ok, "content is %T", res.Messages[0].Content)
	return tc.Text
}

func TestPrompts_SetUpMonitoringGuidesTheFlow(t *testing.T) {
	text := promptText(t, nil)
	for _, p := range []string{
		"Help me set up LastPing monitoring.",
		"Call list_monitors",
		"Ask me what I run unattended: my AI agent sessions (for example Claude Code)",
		"no-code automations (n8n, Zapier, Make), cron jobs and CI workflows",
		"Propose one monitor for each",
		"wait for my agreement",
		"After I agree, create them with create_monitor.",
		"call get_ping_instructions and add exactly what it returns",
		"Ask me before creating, changing or deleting anything.",
	} {
		require.Contains(t, text, p)
	}
	require.NotContains(t, text, "I want to monitor:")
	require.NotContains(t, text, "—")

	// The argument, when given, names the first thing to monitor.
	with := promptText(t, map[string]string{"what": "the backup cron job"})
	require.Contains(t, with, "I want to monitor: the backup cron job.")
	require.Contains(t, with, "Ask me whether anything else runs unattended")
	require.True(t, strings.Contains(with, "create_monitor"))
}

// This binary signs in with an API key only, so the description names the
// key scopes that get the 403 and the one that works.
func TestListAPIKeys_DescriptionSaysOnlyAnAdminKeyCanCallIt(t *testing.T) {
	var res mcp.ListToolsResult
	require.NoError(t, json.Unmarshal(rpc(t, "tools/list", map[string]any{}), &res))
	for _, tl := range res.Tools {
		if tl.Name == "list_api_keys" {
			require.Contains(t, tl.Description, "admin scope")
			require.Contains(t, tl.Description, "A read or write API key gets 403 here; an admin API key works.")
			require.NotContains(t, tl.Description, "OAuth")
			return
		}
	}
	t.Fatal("list_api_keys not served")
}
