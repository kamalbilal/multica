-- name: CreateIssueDebugSession :one
INSERT INTO issue_debug_session (
    id, workspace_id, issue_id, agent_id, source_task_id, ingest_token, status
) VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @workspace_id, @issue_id, @agent_id, sqlc.narg(source_task_id),
    @ingest_token, COALESCE(sqlc.narg('status')::text, 'instrumenting')
)
RETURNING *;

-- name: GetIssueDebugSession :one
SELECT * FROM issue_debug_session
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetOpenIssueDebugSession :one
SELECT * FROM issue_debug_session
WHERE issue_id = @issue_id AND agent_id = @agent_id AND workspace_id = @workspace_id AND status <> 'closed'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetOpenIssueDebugSessionForIssue :one
SELECT * FROM issue_debug_session
WHERE issue_id = @issue_id AND workspace_id = @workspace_id AND status <> 'closed'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListOpenIssueDebugSessions :many
SELECT * FROM issue_debug_session
WHERE issue_id = @issue_id AND workspace_id = @workspace_id AND status <> 'closed'
ORDER BY created_at DESC;

-- name: UpdateIssueDebugSessionWait :one
UPDATE issue_debug_session SET
    status = @status,
    hypotheses = @hypotheses,
    repro_steps = @repro_steps,
    probe_paths = @probe_paths,
    wait_comment_id = sqlc.narg(wait_comment_id),
    source_task_id = COALESCE(sqlc.narg(source_task_id), source_task_id),
    work_dir_hint = @work_dir_hint,
    revision = revision + 1,
    updated_at = clock_timestamp()
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'closed'
RETURNING *;

-- name: UpdateIssueDebugSessionContinue :one
UPDATE issue_debug_session SET
    status = @status,
    continue_action = @continue_action,
    continue_comment_id = sqlc.narg(continue_comment_id),
    log_dump = COALESCE(sqlc.narg(log_dump), log_dump),
    source_task_id = COALESCE(sqlc.narg(source_task_id), source_task_id),
    revision = revision + 1,
    updated_at = clock_timestamp()
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'closed'
RETURNING *;

-- name: UpdateIssueDebugSessionLogDump :one
UPDATE issue_debug_session SET
    log_dump = @log_dump,
    event_count = GREATEST(event_count, @event_count),
    updated_at = clock_timestamp()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: UpdateIssueDebugSessionEventCount :one
UPDATE issue_debug_session SET
    event_count = GREATEST(event_count, @event_count),
    updated_at = clock_timestamp()
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'closed'
RETURNING *;

-- name: CloseIssueDebugSession :one
UPDATE issue_debug_session SET
    status = 'closed',
    revision = revision + 1,
    updated_at = clock_timestamp()
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'closed'
RETURNING *;
