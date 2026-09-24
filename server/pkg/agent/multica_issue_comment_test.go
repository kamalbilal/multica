package agent

import "testing"

func TestMulticaIssueCommentAddCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command string
		want    bool
	}{
		{"multica issue comment add issue-1 --content-file ./reply.md", true},
		{"./multica issue comment add issue-1 --content-file ./reply.md", true},
		{"/usr/local/bin/multica issue comment add issue-1 --content-file ./reply.md", true},
		{"MULTICA_TOKEN=x multica issue comment add issue-1 --content-file ./reply.md", true},
		{"FOO=1 BAR=2 ./multica issue comment add issue-1", true},
		{`sh -c "multica issue comment add issue-1 --content-file ./reply.md"`, true},
		{`bash -c 'multica issue comment add issue-1'`, true},
		{`/bin/sh -c "multica issue comment add issue-1"`, true},
		{"multica issue get issue-1", false},
		{"echo multica issue comment add issue-1", false},
		{`sh -c "echo multica issue comment add issue-1"`, false},
		{"FOO=bar", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isMulticaIssueCommentAddCommand(tt.command); got != tt.want {
			t.Errorf("isMulticaIssueCommentAddCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestMulticaIssueCommentAddToolIgnoresToolTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"shell", "terminal", true},
		{"execute_bash", "execute_bash", true},
		{"run", "run", true},
	}
	for _, tt := range tests {
		msg := Message{
			Type:   MessageToolUse,
			Tool:   tt.tool,
			CallID: "call-1",
			Input:  map[string]any{"command": "multica issue comment add issue-1 --content-file ./reply.md"},
		}
		if got := isMulticaIssueCommentAddTool(msg); got != tt.want {
			t.Errorf("%s: isMulticaIssueCommentAddTool(tool=%q) = %v, want %v", tt.name, tt.tool, got, tt.want)
		}
	}

	notComment := Message{
		Type:   MessageToolUse,
		Tool:   "terminal",
		CallID: "call-2",
		Input:  map[string]any{"command": "multica issue get issue-1"},
	}
	if isMulticaIssueCommentAddTool(notComment) {
		t.Errorf("isMulticaIssueCommentAddTool should reject a non-comment-add command")
	}

	compound := []string{
		`cd "C:\work\repo"; multica issue comment add issue-1 --content-file ./reply.md`,
		`cd /tmp && multica issue comment add issue-1 --content-file ./reply.md`,
		`cd "C:\Users\kamal\work"; multica issue comment add 01a0 --content-file ./reply.md --output table; if ($LASTEXITCODE -eq 0) { Remove-Item ./reply.md }`,
	}
	for _, command := range compound {
		msg := Message{
			Type:   MessageToolUse,
			Tool:   "terminal",
			CallID: "call-compound",
			Input:  map[string]any{"command": command},
		}
		if !isMulticaIssueCommentAddTool(msg) {
			t.Errorf("isMulticaIssueCommentAddTool(%q) = false, want true", command)
		}
	}
}

func TestMulticaIssueCommentAddToolSucceeded(t *testing.T) {
	t.Parallel()

	successJSON := `{"status":"success","value":{"exitCode":0,"stderr":"Comment added to issue x.\n"}}`
	failJSON := `{"status":"success","value":{"exitCode":1,"stderr":"path resolves outside\n"}}`

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"exit zero", successJSON, true},
		{"exit one", failJSON, false},
		{"legacy posted", "comment posted", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		msg := Message{Type: MessageToolResult, Output: tt.output, Status: "completed"}
		if got := multicaIssueCommentAddToolSucceeded(msg); got != tt.want {
			t.Errorf("%s: multicaIssueCommentAddToolSucceeded() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
