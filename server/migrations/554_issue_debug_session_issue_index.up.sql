CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_debug_session_issue
    ON issue_debug_session (issue_id, created_at DESC);
