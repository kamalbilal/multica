CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_debug_session_source_task
    ON issue_debug_session (source_task_id)
    WHERE source_task_id IS NOT NULL;
