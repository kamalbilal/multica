package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// discoverCursorSdkModels queries the Node executor for Cursor.models.list().
func discoverCursorSdkModels(ctx context.Context, runtimeCmd Command) (Catalog, error) {
	executorPath := strings.TrimSpace(runtimeCmd.Path)
	if executorPath == "" {
		executorPath = strings.TrimSpace(os.Getenv(CursorSdkExecutorEnv))
	}
	if executorPath == "" {
		executorPath = defaultCursorSdkExecutorRelPath
	}
	if _, err := os.Stat(executorPath); err != nil {
		return Catalog{Models: cursorStaticModels(), Fallback: true}, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := newCursorSdkClientForDiscovery(runCtx, executorPath, runtimeCmd)
	if err != nil {
		return Catalog{Models: cursorStaticModels(), Fallback: true}, nil
	}
	defer func() {
		_ = client.Close()
	}()

	items, err := client.ListModels(runCtx, cursorSdkDefaultAPIKeyEnv)
	if err != nil {
		return Catalog{Models: cursorStaticModels(), Fallback: true}, nil
	}

	models := parseCursorSdkModels(items)
	if len(models) == 0 {
		return Catalog{Models: cursorStaticModels(), Fallback: true}, nil
	}
	return Catalog{Models: models}, nil
}

func newCursorSdkClientForDiscovery(ctx context.Context, executorPath string, runtimeCmd Command) (*CursorSdkClient, error) {
	node, script, err := resolveCursorSdkExecutor(executorPath, runtimeCmd.Launcher)
	if err != nil {
		return nil, err
	}
	return newCursorSdkClientWithResolvedPaths(ctx, node, script, runtimeCmd.DiscoveryEnv, runtimeCmd.logger)
}

func parseCursorSdkModels(items []json.RawMessage) []Model {
	var models []Model
	seen := map[string]bool{}
	for _, raw := range items {
		var entry struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
			Default     bool   `json:"default"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		id := strings.TrimSpace(entry.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		label := strings.TrimSpace(entry.DisplayName)
		if label == "" {
			label = strings.TrimSpace(entry.Name)
		}
		if label == "" {
			label = id
		}
		models = append(models, Model{
			ID:       id,
			Label:    label,
			Provider: "cursor",
			Default:  entry.Default,
		})
	}
	return models
}
