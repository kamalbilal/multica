package execenv

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRuntimeConfigFollowDirective(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	want, err := filepath.Abs(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	for _, provider := range []string{"cursor", "cursor_sdk"} {
		got := BuildRuntimeConfigFollowDirective(dir, provider)
		if !strings.Contains(got, want) {
			t.Fatalf("%s directive missing path %q:\n%s", provider, want, got)
		}
		if !strings.Contains(got, "mandatory") {
			t.Fatalf("%s directive missing mandatory wording:\n%s", provider, got)
		}
	}

	if got := BuildRuntimeConfigFollowDirective(dir, "codex"); got != "" {
		t.Fatalf("codex should not get follow directive, got:\n%s", got)
	}
	if got := BuildRuntimeConfigFollowDirective("", "cursor_sdk"); got != "" {
		t.Fatalf("empty workdir should omit directive, got:\n%s", got)
	}
}
