package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProbeCursorSdkRequiresNode22AndExecutor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("node --version probe is exercised on unix hosts")
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}

	executorDir := t.TempDir()
	executor := filepath.Join(executorDir, "cli.js")
	if err := os.WriteFile(executor, []byte("export {};\n"), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}

	t.Setenv("MULTICA_CURSOR_SDK_EXECUTOR", executor)
	t.Setenv("MULTICA_CURSOR_SDK_MODEL", "composer-2")

	out, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Fatalf("node --version: %v", err)
	}
	major := nodeVersionMajorRe.FindStringSubmatch(string(out))
	if len(major) < 2 {
		t.Fatalf("parse node version from %q", out)
	}

	entry, ok := probeCursorSdk()
	if major[1] >= "22" {
		if !ok {
			t.Fatal("probeCursorSdk() = false, want true when node >= 22 and executor exists")
		}
		if entry.Path != executor {
			t.Fatalf("entry.Path = %q, want %q", entry.Path, executor)
		}
		if entry.Command != node {
			t.Fatalf("entry.Command = %q, want %q", entry.Command, node)
		}
		if entry.Model != "composer-2" {
			t.Fatalf("entry.Model = %q, want composer-2", entry.Model)
		}
	} else if ok {
		t.Fatalf("probeCursorSdk() = true on node %s, want false below major 22", string(out))
	}
}

func TestProbeAgentCLIsDiscoversCursorSdkWhenAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("node --version probe is exercised on unix hosts")
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	out, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Fatalf("node --version: %v", err)
	}
	major := nodeVersionMajorRe.FindStringSubmatch(string(out))
	if len(major) < 2 || major[1] < "22" {
		t.Skip("node major version below 22")
	}

	executorDir := t.TempDir()
	executor := filepath.Join(executorDir, "cli.js")
	if err := os.WriteFile(executor, []byte("export {};\n"), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}
	t.Setenv("MULTICA_CURSOR_SDK_EXECUTOR", executor)

	agents := probeAgentCLIs()
	entry, ok := agents["cursor_sdk"]
	if !ok {
		t.Fatal("cursor_sdk was not discovered by probeAgentCLIs")
	}
	if entry.Path != executor {
		t.Fatalf("cursor_sdk path = %q, want %q", entry.Path, executor)
	}
}

func TestProbeCursorSdkMissingExecutorIsNotDiscovered(t *testing.T) {
	t.Setenv("MULTICA_CURSOR_SDK_EXECUTOR", filepath.Join(t.TempDir(), "missing-cli.js"))
	if _, ok := probeCursorSdk(); ok {
		t.Fatal("probeCursorSdk() = true for missing executor, want false")
	}
}
