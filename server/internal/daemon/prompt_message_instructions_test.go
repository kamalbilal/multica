package daemon

import (
	"strings"
	"testing"
)

func TestBuildPromptPrependsStandingMessageInstructions(t *testing.T) {
	t.Parallel()

	const standing = "Always reply in bullets."
	agent := &AgentData{Name: "Worker", MessageInstructions: standing}

	t.Run("human comment", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-1",
			TriggerCommentContent: "Please fix the parser.",
			TriggerAuthorType:     "member",
			TriggerAuthorName:     "Ada",
			Agent:                 agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "Please fix the parser.")
	})

	t.Run("agent-authored comment", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-2",
			TriggerCommentContent: "Please implement the follow-up.",
			TriggerAuthorType:     "agent",
			TriggerAuthorName:     "Leader",
			Agent:                 agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "Please implement the follow-up.")
		if !strings.Contains(out, "Another agent (Leader)") {
			t.Fatalf("prompt lost agent author label:\n%s", out)
		}
	})

	t.Run("coalesced comments", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-3",
			TriggerCommentContent: "Latest ask.",
			TriggerAuthorType:     "member",
			CoalescedComments: []CoalescedCommentData{{
				ID:         "comment-1",
				Content:    "Earlier ask.",
				AuthorType: "agent",
				AuthorName: "Leader",
			}},
			Agent: agent,
		}, "claude")
		if n := strings.Count(out, standingMessageInstructionsHeading); n != 2 {
			t.Fatalf("expected 2 standing headings (trigger + coalesced), got %d:\n%s", n, out)
		}
		assertStandingPrefixesMessage(t, out, standing, "Latest ask.")
		assertStandingPrefixesMessage(t, out, standing, "Earlier ask.")
	})

	t.Run("assignment handoff", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:     "issue-1",
			HandoffNote: "Scope to the auth module only.",
			Agent:       agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "Scope to the auth module only.")
	})

	t.Run("wakeup instruction", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:     "issue-1",
			WakeupID:    "wake-1",
			HandoffNote: "Check CI is green.",
			Agent:       agent,
		}, "claude")
		if !strings.Contains(out, "[WAKEUP]") {
			t.Fatalf("missing wakeup marker:\n%s", out)
		}
		assertStandingPrefixesMessage(t, out, standing, "Check CI is green.")
	})

	t.Run("quick create user input", func(t *testing.T) {
		out := BuildPrompt(Task{
			QuickCreatePrompt: "Add dark mode toggle",
			Agent:             agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "Add dark mode toggle")
	})

	t.Run("autopilot trigger payload", func(t *testing.T) {
		out := BuildPrompt(Task{
			AutopilotRunID:          "run-1",
			AutopilotTriggerPayload: []byte("deploy staging now"),
			Agent:                   agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "deploy staging now")
	})

	t.Run("chat", func(t *testing.T) {
		out := BuildPrompt(Task{
			ChatSessionID: "sess-1",
			ChatMessage:   "how does the parser work?",
			Agent:         agent,
		}, "claude")
		assertStandingPrefixesMessage(t, out, standing, "how does the parser work?")
	})

	t.Run("assignment without inbound message omits standing", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID: "issue-1",
			Agent:   agent,
		}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("assignment without inbound message should not prefix:\n%s", out)
		}
	})

	t.Run("omitted when empty", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-1",
			TriggerCommentContent: "Please fix the parser.",
			Agent:                 &AgentData{Name: "Worker"},
		}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("empty field still prefixed:\n%s", out)
		}
	})

	t.Run("omitted when whitespace", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-1",
			TriggerCommentContent: "Please fix the parser.",
			Agent:                 &AgentData{MessageInstructions: "  \n\t  "},
		}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("whitespace-only field still prefixed:\n%s", out)
		}
	})

	t.Run("omitted when agent payload is missing", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID:               "issue-1",
			TriggerCommentID:      "comment-1",
			TriggerCommentContent: "Please fix the parser.",
		}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("nil agent still prefixed:\n%s", out)
		}
	})
}

func assertStandingPrefixesMessage(t *testing.T, out, standing, message string) {
	t.Helper()
	mi := strings.Index(out, message)
	if mi < 0 {
		t.Fatalf("inbound message %q missing:\n%s", message, out)
	}
	before := out[:mi]
	hi := strings.LastIndex(before, standingMessageInstructionsHeading)
	if hi < 0 {
		t.Fatalf("standing heading must precede message %q:\n%s", message, out)
	}
	si := strings.LastIndex(before, standing)
	if si < hi {
		t.Fatalf("standing text must follow heading before message %q:\n%s", message, out)
	}
	// Standing instructions must sit on the same inbound payload, not only in boilerplate.
	between := strings.TrimSpace(out[si+len(standing) : mi])
	if strings.Contains(between, "You are running as a local coding agent") {
		t.Fatalf("standing instructions were not prepended onto inbound message %q:\n%s", message, out)
	}
}

func TestBlockquoteInboundStandingMultiline(t *testing.T) {
	t.Parallel()

	const standing = "Line one.\nLine two."
	quoted := blockquoteInbound(standing, "User\ncomment")
	if !strings.Contains(quoted, standingMessageInstructionsHeading) {
		t.Fatalf("heading missing:\n%s", quoted)
	}
	if !strings.Contains(quoted, "> Line one.") || !strings.Contains(quoted, "> Line two.") {
		t.Fatalf("multiline standing not continued:\n%s", quoted)
	}
	if !strings.Contains(quoted, "> User") || !strings.Contains(quoted, "> comment") {
		t.Fatalf("multiline comment not continued:\n%s", quoted)
	}
}

func TestFormatTaskSupplementInstructionPrefixesStanding(t *testing.T) {
	t.Parallel()

	plain := formatTaskSupplementInstruction("Ada", "Create evidence.txt", "")
	if strings.Contains(plain, standingMessageInstructionsHeading) {
		t.Fatalf("empty field still prefixed:\n%s", plain)
	}
	if !strings.Contains(plain, "Create evidence.txt") {
		t.Fatalf("lost steer content:\n%s", plain)
	}

	prefixed := formatTaskSupplementInstruction("Ada", "Create evidence.txt", "Always reply in bullets.")
	if !strings.Contains(prefixed, standingMessageInstructionsHeading) {
		t.Fatalf("heading missing:\n%s", prefixed)
	}
	if !strings.Contains(prefixed, "Always reply in bullets.") {
		t.Fatalf("standing text missing:\n%s", prefixed)
	}
	idxStanding := strings.Index(prefixed, "Always reply in bullets.")
	idxMessage := strings.Index(prefixed, "Create evidence.txt")
	if idxStanding > idxMessage {
		t.Fatalf("standing must precede steered message:\n%s", prefixed)
	}
	if !strings.Contains(prefixed, "Human message:") {
		t.Fatalf("lost steer wrapper:\n%s", prefixed)
	}
}
