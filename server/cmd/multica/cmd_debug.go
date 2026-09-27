package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon"
	"github.com/multica-ai/multica/server/internal/daemon/debugingest"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newDebugCommand()) }

func newDebugCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "debug", Short: "Issue debug sessions: wait, status, close"}
	cmd.PersistentFlags().String("issue", "", "Issue id (defaults to MULTICA_ISSUE_ID)")
	cmd.AddCommand(newDebugWaitCommand())
	cmd.AddCommand(newDebugStatusCommand())
	cmd.AddCommand(newDebugCloseCommand())
	return cmd
}

func newDebugWaitCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "wait",
		Short: "Park this debug turn for a human retest (does not block)",
		Args:  cobra.NoArgs,
		RunE:  runDebugWait,
	}
	c.Flags().String("wait-comment-id", "", "Comment that lists hypotheses and retest steps")
	c.Flags().String("repro-steps", "", "Exact steps the human should follow")
	c.Flags().String("hypotheses", "[]", "JSON array of hypotheses")
	c.Flags().String("probe-paths", "[]", "JSON array of instrumented paths")
	c.Flags().String("status", "waiting_repro", "waiting_repro or waiting_verify")
	c.Flags().String("work-dir-hint", "", "Directory the human should retest in")
	return c
}

func newDebugStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show the open debug session", Args: cobra.NoArgs, RunE: runDebugStatus}
}

func newDebugCloseCommand() *cobra.Command {
	return &cobra.Command{Use: "close", Short: "Close the debug session after probes are removed", Args: cobra.NoArgs, RunE: runDebugClose}
}

func debugIssueRef(cmd *cobra.Command) (string, error) {
	issue, _ := cmd.Flags().GetString("issue")
	if issue == "" {
		issue = strings.TrimSpace(os.Getenv("MULTICA_ISSUE_ID"))
	}
	if issue == "" {
		return "", fmt.Errorf("pass --issue or set MULTICA_ISSUE_ID")
	}
	return issue, nil
}

func runDebugWait(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	issue, err := debugIssueRef(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	ref, err := resolveIssueRef(ctx, client, issue)
	if err != nil {
		return err
	}
	waitCommentID, _ := cmd.Flags().GetString("wait-comment-id")
	repro, _ := cmd.Flags().GetString("repro-steps")
	status, _ := cmd.Flags().GetString("status")
	workDir, _ := cmd.Flags().GetString("work-dir-hint")
	if strings.TrimSpace(workDir) == "" {
		if cwd, err := os.Getwd(); err == nil {
			workDir = cwd
		}
	}
	hypRaw, _ := cmd.Flags().GetString("hypotheses")
	pathRaw, _ := cmd.Flags().GetString("probe-paths")
	var hypotheses json.RawMessage
	if err := json.Unmarshal([]byte(hypRaw), &hypotheses); err != nil {
		return fmt.Errorf("--hypotheses must be JSON: %w", err)
	}
	var probePaths json.RawMessage
	if err := json.Unmarshal([]byte(pathRaw), &probePaths); err != nil {
		return fmt.Errorf("--probe-paths must be JSON: %w", err)
	}
	body := map[string]any{
		"hypotheses":      hypotheses,
		"repro_steps":     repro,
		"probe_paths":     probePaths,
		"wait_comment_id": waitCommentID,
		"work_dir_hint":   workDir,
		"status":          status,
	}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+ref.ID+"/debug/wait", body, &result); err != nil {
		return err
	}
	if sessionID := strings.TrimSpace(os.Getenv("MULTICA_DEBUG_SESSION_ID")); sessionID != "" {
		if root := strings.TrimSpace(os.Getenv(daemon.TaskWorkspacesRootEnv)); root != "" {
			token := strings.TrimSpace(os.Getenv("MULTICA_DEBUG_INGEST_TOKEN"))
			if err := debugingest.WriteHoldMeta(root, sessionID, ref.ID, token); err != nil {
				return fmt.Errorf("write debug hold file: %w", err)
			}
		}
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runDebugStatus(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	issue, err := debugIssueRef(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	ref, err := resolveIssueRef(ctx, client, issue)
	if err != nil {
		return err
	}
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+ref.ID+"/debug", &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runDebugClose(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	issue, err := debugIssueRef(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	ref, err := resolveIssueRef(ctx, client, issue)
	if err != nil {
		return err
	}
	if sessionID := strings.TrimSpace(os.Getenv("MULTICA_DEBUG_SESSION_ID")); sessionID != "" {
		if root := strings.TrimSpace(os.Getenv(daemon.TaskWorkspacesRootEnv)); root != "" {
			debugingest.ClearHold(root, sessionID)
		}
	}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+ref.ID+"/debug/close", map[string]any{}, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}
