package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProbeBuiltinRuntimeCursorSdkSkipsScriptVersionProbe(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}

	executor := filepath.Join(t.TempDir(), "cli.js")
	if err := os.WriteFile(executor, []byte("export {};\n"), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}

	entry := AgentEntry{Path: executor, Command: node}
	d := freshDaemon("")
	d.cfg.Agents = map[string]AgentEntry{"cursor_sdk": entry}

	version, reason, verdict := d.probeBuiltinRuntime(context.Background(), "cursor_sdk", entry)
	if verdict != builtinProbeOK {
		t.Fatalf("verdict = %v (%q), want builtinProbeOK", verdict, reason)
	}
	if version == "" {
		t.Fatal("version is empty, want synthetic bundled version")
	}
}

func TestResolveAgentEntryForLaunchCursorSdkSkipsScriptVersionProbe(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}

	executor := filepath.Join(t.TempDir(), "cli.js")
	if err := os.WriteFile(executor, []byte("export {};\n"), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}

	entry := AgentEntry{Path: executor, Command: node}
	d := freshDaemon("")
	d.cfg.Agents = map[string]AgentEntry{"cursor_sdk": entry}

	got, version, err := d.resolveAgentEntryForLaunch(context.Background(), "cursor_sdk", entry)
	if err != nil {
		t.Fatalf("resolveAgentEntryForLaunch: %v", err)
	}
	if got.Path != executor {
		t.Fatalf("path = %q, want %q", got.Path, executor)
	}
	if version != cursorSdkSyntheticVersion {
		t.Fatalf("version = %q, want %q", version, cursorSdkSyntheticVersion)
	}
}
