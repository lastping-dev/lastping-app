package mcptools

// firstrun.go: what an assistant is told when a project is still empty, and
// the set_up_monitoring prompt.
//
// WHY THE OFFER IS IN THE RESULT. The server Instructions already say to offer
// setup when list_monitors comes back empty, but an assistant that calls
// list_monitors, list_agents and list_destinations on an empty project and
// reads "No monitors found." tends to stop there: a model weighs a tool result
// far above server instructions it read once at initialize. So the empty
// result itself says what a useful next step is, keeping the same rule the
// Instructions end on: nothing is created until the person agrees.
//
// These texts are results, not tool descriptions, so the directory policy's
// description rules (policy_test.go) do not bind them; they still stay plain:
// one offer, made once, and every write waits for the person.
//
// The texts match the hosted server at mcp.lastping.dev word for word.

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// unattendedWork is the message spine's list of what LastPing watches, shared
// by the empty results and the prompt so they cannot name different things.
const unattendedWork = "their AI agent sessions (for example Claude Code), scheduled AI tasks and routines, " +
	"no-code automations (n8n, Zapier, Make), cron jobs and CI workflows"

// EmptyMonitorsText is list_monitors' result for a project with no monitors.
const EmptyMonitorsText = "This project has no monitors yet. If the user hasn't asked for something else, " +
	"offer once to set up monitoring for what they run unattended: " + unattendedWork + ". " +
	"Propose one monitor for each. Only after they agree, call create_monitor, " +
	"then get_ping_instructions for exactly what to add."

// EmptyAgentsText is list_agents' result for a project with no agents.
const EmptyAgentsText = "This project has no agents yet. An agent groups the monitors of one AI agent or autonomous worker, " +
	"for example a Claude Code setup or a scheduled AI task. If the user hasn't asked for something else, " +
	"offer once to set up monitoring for the AI agents they run, with one monitor for each. " +
	"Only after they agree, call create_monitor (and register_agent first when they want those monitors grouped under a named agent), " +
	"then get_ping_instructions for exactly what to add."

// EmptyDestinationsText is list_destinations' result for a project with no
// destinations. It states the consequence (alerts reach nobody) and does not
// promise the default email destination: a project reaches this state exactly
// when that destination is absent, for whatever reason.
const EmptyDestinationsText = "This project has no alert destinations, so alerts from its monitors are not delivered anywhere. " +
	"If the user hasn't asked for something else, offer once to add one where they want to be alerted: " +
	"email, Slack, Discord, a webhook or another kind create_destination lists. " +
	"Only after they choose, call create_destination, then set_route to send a monitor's alerts there. " +
	"An email destination receives alerts once its confirmation link is clicked."

// SetUpMonitoringPrompt is the name of the guided-setup prompt.
const SetUpMonitoringPrompt = "set_up_monitoring"

// setUpMonitoringBody is the prompt's message, written as the person asking.
// what is empty unless the person named what to monitor.
func setUpMonitoringBody(what string) string {
	var b strings.Builder
	b.WriteString("Help me set up LastPing monitoring. LastPing alerts me when my AI agents and automations fail, " +
		"stall, wait for my approval or never run, and it watches cron jobs and CI/CD pipelines the same way.\n\n")
	b.WriteString("1. Call list_monitors to see what this project already watches.\n")
	if what = strings.TrimSpace(what); what != "" {
		b.WriteString("2. I want to monitor: " + what + ". Ask me whether anything else runs unattended: " + unattendedWorkMine + ".\n")
	} else {
		b.WriteString("2. Ask me what I run unattended: " + unattendedWorkMine + ".\n")
	}
	b.WriteString("3. Propose one monitor for each, with a name and how often it runs, and wait for my agreement.\n")
	b.WriteString("4. After I agree, create them with create_monitor.\n")
	b.WriteString("5. For each one, call get_ping_instructions and add exactly what it returns. " +
		"Where it is a change only I can make, show me the steps.\n\n")
	b.WriteString("Ask me before creating, changing or deleting anything.")
	return b.String()
}

// unattendedWorkMine is unattendedWork in the first person, for the prompt.
const unattendedWorkMine = "my AI agent sessions (for example Claude Code), scheduled AI tasks and routines, " +
	"no-code automations (n8n, Zapier, Make), cron jobs and CI workflows"

func registerPrompts(s *server.MCPServer) {
	s.AddPrompt(
		mcp.NewPrompt(SetUpMonitoringPrompt,
			mcp.WithPromptTitle("Set up LastPing monitoring"),
			mcp.WithPromptDescription("Guided setup: find what runs unattended (AI agent sessions, scheduled AI tasks, "+
				"no-code automations, cron jobs, CI workflows), agree on one monitor for each, create them and add their pings."),
			mcp.WithArgument("what",
				mcp.ArgumentDescription("Optional. What to monitor, for example 'my nightly Claude Code routine' or 'the backup cron job'.")),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			notePromptFetch(ctx, SetUpMonitoringPrompt)
			return mcp.NewGetPromptResult(
				"Set up LastPing monitoring",
				[]mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(setUpMonitoringBody(req.Params.Arguments["what"]))),
				},
			), nil
		},
	)
}
