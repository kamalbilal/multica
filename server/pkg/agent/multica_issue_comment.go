package agent

import (
	"encoding/json"
	"strings"
)

// isMulticaIssueCommentAddTool reports whether a tool-use message is a
// `multica issue comment add` invocation. It keys purely off the command
// payload, not the normalized tool name.
func isMulticaIssueCommentAddTool(msg Message) bool {
	command, _ := msg.Input["command"].(string)
	if isMulticaIssueCommentAddCommand(command) {
		return true
	}
	for _, segment := range shellCommandSegments(command) {
		if isMulticaIssueCommentAddCommand(segment) {
			return true
		}
	}
	return false
}

// multicaIssueCommentAddToolSucceeded reports whether a tool_result indicates the
// comment was actually posted (not a failed shell invocation).
func multicaIssueCommentAddToolSucceeded(msg Message) bool {
	if !cursorSdkToolResultSucceeded(msg) {
		return false
	}
	exitCode, ok := shellToolResultExitCode(msg.Output)
	if ok {
		return exitCode == 0
	}
	out := strings.ToLower(strings.TrimSpace(msg.Output))
	if out == "" {
		return false
	}
	if strings.Contains(out, "comment added") {
		return true
	}
	return !strings.Contains(out, "error") && !strings.Contains(out, "failed")
}

func shellToolResultExitCode(output string) (int, bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, false
	}
	var wrapper struct {
		Value struct {
			ExitCode int `json:"exitCode"`
		} `json:"value"`
	}
	if err := json.Unmarshal([]byte(output), &wrapper); err != nil {
		return 0, false
	}
	return wrapper.Value.ExitCode, true
}

// shellCommandSegments splits a compound shell command on ;, &&, and || outside quotes.
func shellCommandSegments(command string) []string {
	var segments []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(c)
		case !inSingle && !inDouble && c == ';':
			appendShellSegment(&segments, &current)
		case !inSingle && !inDouble && i+1 < len(command) && c == '&' && command[i+1] == '&':
			appendShellSegment(&segments, &current)
			i++
		case !inSingle && !inDouble && i+1 < len(command) && c == '|' && command[i+1] == '|':
			appendShellSegment(&segments, &current)
			i++
		default:
			current.WriteByte(c)
		}
	}
	appendShellSegment(&segments, &current)
	return segments
}

func appendShellSegment(segments *[]string, current *strings.Builder) {
	segment := strings.TrimSpace(current.String())
	if segment != "" {
		*segments = append(*segments, segment)
	}
	current.Reset()
}

func isMulticaIssueCommentAddCommand(command string) bool {
	parts := trimLeadingEnvAssignments(strings.Fields(command))
	// Some runtimes route terminal calls through a shell wrapper such as
	// `sh -c "multica issue comment add ..."`; unwrap a single such layer so
	// the real invocation is still recognized.
	if len(parts) >= 3 && isPOSIXShellName(parts[0]) && parts[1] == "-c" {
		inner := strings.Trim(strings.Join(parts[2:], " "), "\"'")
		parts = trimLeadingEnvAssignments(strings.Fields(inner))
	}
	if len(parts) < 4 {
		return false
	}
	executable := strings.TrimPrefix(parts[0], "./")
	if executable != "multica" && !strings.HasSuffix(executable, "/multica") {
		return false
	}
	return parts[1] == "issue" && parts[2] == "comment" && parts[3] == "add"
}

// trimLeadingEnvAssignments drops leading `KEY=VALUE` tokens so an invocation
// like `MULTICA_TOKEN=x multica issue comment add ...` is still recognized.
func trimLeadingEnvAssignments(parts []string) []string {
	for len(parts) > 0 && isEnvAssignment(parts[0]) {
		parts = parts[1:]
	}
	return parts
}

// isEnvAssignment reports whether tok is a `NAME=value` shell env assignment.
func isEnvAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := tok[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// isPOSIXShellName reports whether tok names a shell that takes `-c <command>`.
func isPOSIXShellName(tok string) bool {
	if i := strings.LastIndexByte(tok, '/'); i >= 0 {
		tok = tok[i+1:]
	}
	switch tok {
	case "sh", "bash", "zsh", "dash":
		return true
	default:
		return false
	}
}
