package daemon

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPromptCursorFamilyIncludesRuntimeConfigDirective(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	out := BuildPrompt(Task{IssueID: "issue-1"}, "cursor_sdk", WithAgentWorkDir(workDir))
	if !strings.Contains(out, "## Multica runtime config (mandatory)") {
		t.Fatalf("missing runtime config directive:\n%s", out)
	}
	want, err := filepath.Abs(filepath.Join(workDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("expected AGENTS.md path %q in prompt:\n%s", want, out)
	}
	idx := strings.Index(out, "## Multica runtime config")
	issueIdx := strings.Index(out, "issue-1")
	if idx < 0 || issueIdx < 0 || idx > issueIdx {
		t.Fatalf("directive must precede task body (directive@%d issue@%d):\n%s", idx, issueIdx, out)
	}
}

func TestBuildPromptCursorFamilyProvidersIncludeRuntimeConfigDirective(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	want, err := filepath.Abs(filepath.Join(workDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for _, provider := range []string{"cursor", "cursor_sdk"} {
		out := BuildPrompt(Task{IssueID: "issue-1"}, provider, WithAgentWorkDir(workDir))
		if !strings.Contains(out, want) {
			t.Fatalf("%s missing AGENTS.md path %q:\n%s", provider, want, out)
		}
	}
}

func TestBuildPromptNonCursorOmitsRuntimeConfigDirective(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{"claude", "codex"} {
		out := BuildPrompt(Task{IssueID: "issue-1"}, provider, WithAgentWorkDir("/tmp/work"))
		if strings.Contains(out, "## Multica runtime config (mandatory)") {
			t.Fatalf("%s prompt should not carry cursor-family directive:\n%s", provider, out)
		}
	}
}

func TestBuildPromptDebugSessionBlock(t *testing.T) {
	t.Parallel()

	out := BuildPrompt(Task{
		IssueID: "issue-1",
		DebugSession: &DebugSessionData{
			ID:     "sess-1",
			Status: "instrumenting",
		},
	}, "claude")
	if !strings.Contains(out, "## Debug session") {
		t.Fatalf("missing debug session block:\n%s", out)
	}
	if !strings.Contains(out, "multica debug wait") {
		t.Fatalf("missing wait command:\n%s", out)
	}
	if !strings.Contains(out, "multica-debug: start") {
		t.Fatalf("missing probe markers:\n%s", out)
	}

	if !strings.Contains(out, "--work-dir-hint") {
		t.Fatalf("missing work-dir-hint on wait:\n%s", out)
	}
	if !strings.Contains(out, "MULTICA_DEBUG_LOG_PATH") {
		t.Fatalf("missing CORS ladder log path:\n%s", out)
	}

	withURL := BuildPrompt(Task{
		IssueID: "issue-1",
		DebugSession: &DebugSessionData{
			ID:        "sess-1",
			Status:    "instrumenting",
			IngestURL: "http://127.0.0.1:59999/ingest/tok-1",
		},
	}, "claude")
	if !strings.Contains(withURL, "http://127.0.0.1:59999/ingest/tok-1") {
		t.Fatalf("prompt must include the concrete ingest URL:\n%s", withURL)
	}
	if !strings.Contains(withURL, "hardcode") {
		t.Fatalf("prompt must tell the agent to hardcode the ingest URL:\n%s", withURL)
	}

	continued := BuildPrompt(Task{
		IssueID: "issue-1",
		DebugSession: &DebugSessionData{
			ID:             "sess-1",
			Status:         "analyzing",
			ContinueAction: "reproduced",
		},
	}, "claude")
	if !strings.Contains(continued, "MULTICA_DEBUG_LOG_PATH") {
		t.Fatalf("empty continue dump must keep CORS ladder:\n%s", continued)
	}
	if !strings.Contains(out, "never ship a credentialed request") {
		t.Fatalf("missing CORS ladder credentials rule:\n%s", out)
	}

	fixed := BuildPrompt(Task{
		IssueID: "issue-1",
		DebugSession: &DebugSessionData{
			ID:             "sess-1",
			Status:         "analyzing",
			ContinueAction: "fixed",
		},
	}, "claude")
	if !strings.Contains(fixed, "multica debug close") {
		t.Fatalf("fixed continue should tell the agent to close:\n%s", fixed)
	}
	if !strings.Contains(fixed, "Grep the tree until zero") {
		t.Fatalf("fixed continue should require marker cleanup:\n%s", fixed)
	}

	plain := BuildPrompt(Task{IssueID: "issue-1"}, "claude")
	if strings.Contains(plain, "## Debug session") {
		t.Fatalf("non-debug prompt should not carry debug block:\n%s", plain)
	}
}
