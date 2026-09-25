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
		assertStandingPrefix(t, out, standing)
		if !strings.Contains(out, "Please fix the parser.") {
			t.Fatalf("prompt lost the human comment:\n%s", out)
		}
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
		assertStandingPrefix(t, out, standing)
		if !strings.Contains(out, "Please implement the follow-up.") {
			t.Fatalf("prompt lost the agent comment:\n%s", out)
		}
	})

	t.Run("chat", func(t *testing.T) {
		out := BuildPrompt(Task{
			ChatSessionID: "sess-1",
			ChatMessage:   "how does the parser work?",
			Agent:         agent,
		}, "claude")
		assertStandingPrefix(t, out, standing)
		if !strings.Contains(out, "how does the parser work?") {
			t.Fatalf("prompt lost the chat message:\n%s", out)
		}
	})

	t.Run("assignment", func(t *testing.T) {
		out := BuildPrompt(Task{
			IssueID: "issue-1",
			Agent:   agent,
		}, "claude")
		assertStandingPrefix(t, out, standing)
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
			IssueID: "issue-1",
			Agent:   &AgentData{MessageInstructions: "  \n\t  "},
		}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("whitespace-only field still prefixed:\n%s", out)
		}
	})

	t.Run("omitted when agent payload is missing", func(t *testing.T) {
		out := BuildPrompt(Task{IssueID: "issue-1"}, "claude")
		if strings.Contains(out, standingMessageInstructionsHeading) {
			t.Fatalf("nil agent still prefixed:\n%s", out)
		}
	})
}

func assertStandingPrefix(t *testing.T, out, standing string) {
	t.Helper()
	heading := standingMessageInstructionsHeading
	hi := strings.Index(out, heading)
	if hi != 0 {
		t.Fatalf("standing heading is not at the start of the prompt (index %d):\n%s", hi, out)
	}
	si := strings.Index(out, standing)
	if si < 0 {
		t.Fatalf("standing text missing:\n%s", out)
	}
	if si < hi {
		t.Fatalf("standing text appeared before its heading")
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
	if !strings.HasPrefix(prefixed, standingMessageInstructionsHeading+"\n\nAlways reply in bullets.\n\n") {
		t.Fatalf("prefix missing or not at start:\n%s", prefixed)
	}
	if !strings.Contains(prefixed, "Create evidence.txt") {
		t.Fatalf("lost steer content after prefix:\n%s", prefixed)
	}
}
