CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_debug_session_open
    ON issue_debug_session (issue_id, agent_id)
    WHERE status <> 'closed';
