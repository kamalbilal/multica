package execenv

// IsCursorFamilyProvider reports whether provider uses Cursor-style filesystem
// prep: AGENTS.md brief, .cursor/skills discovery, and managed MCP sidecars.
func IsCursorFamilyProvider(provider string) bool {
	return provider == "cursor" || provider == "cursor_sdk"
}
