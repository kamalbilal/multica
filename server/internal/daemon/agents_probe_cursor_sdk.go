package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/pkg/agent"
)

const cursorSdkSyntheticVersion = "0.0.0"

var nodeVersionMajorRe = regexp.MustCompile(`v?(\d+)\.`)

// probeCursorSdk discovers the bundled @cursor/sdk Node executor when Node 22+
// is available and the executor script exists on disk.
func probeCursorSdk() (AgentEntry, bool) {
	override := strings.TrimSpace(os.Getenv(agent.CursorSdkExecutorEnv))
	node, script, err := agent.ResolveCursorSdkExecutor(override)
	if err != nil {
		return AgentEntry{}, false
	}
	if !nodeMajorVersionAtLeast(node, 22) {
		return AgentEntry{}, false
	}
	return AgentEntry{
		Path:    script,
		Command: node,
		Model:   strings.TrimSpace(os.Getenv("MULTICA_CURSOR_SDK_MODEL")),
	}, true
}

func nodeMajorVersionAtLeast(nodePath string, minMajor int) bool {
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return false
	}
	m := nodeVersionMajorRe.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(m) < 2 {
		return false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return false
	}
	return major >= minMajor
}

func cursorSdkScriptPresent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// verifyCursorSdkAgentEntry checks that node and the executor script are usable.
// The executor is launched as `node <script>` over JSONL and has no --version flag.
func verifyCursorSdkAgentEntry(entry AgentEntry) (string, error) {
	if strings.TrimSpace(entry.Command) == "" {
		return "", errors.New("cursor sdk executor requires node on PATH")
	}
	if !cursorSdkScriptPresent(entry.Path) {
		return "", fmt.Errorf("cursor sdk executor script not found at %s", entry.Path)
	}
	return cursorSdkSyntheticVersion, nil
}
