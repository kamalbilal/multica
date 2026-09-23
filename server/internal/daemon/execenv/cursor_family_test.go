package execenv

import "testing"

func TestIsCursorFamilyProvider(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{"cursor", "cursor_sdk"} {
		if !IsCursorFamilyProvider(provider) {
			t.Fatalf("IsCursorFamilyProvider(%q) = false, want true", provider)
		}
	}
	for _, provider := range []string{"", "claude", "codex", "cursor-agent"} {
		if IsCursorFamilyProvider(provider) {
			t.Fatalf("IsCursorFamilyProvider(%q) = true, want false", provider)
		}
	}
}
