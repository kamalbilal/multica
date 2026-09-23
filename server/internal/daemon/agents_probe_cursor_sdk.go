package daemon

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/pkg/agent"
)

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
