CREATE TABLE issue_debug_session (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    issue_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    source_task_id uuid,
    wait_comment_id uuid,
    status text NOT NULL DEFAULT 'instrumenting'
        CHECK (status IN ('instrumenting', 'waiting_repro', 'analyzing', 'waiting_verify', 'closed')),
    hypotheses jsonb NOT NULL DEFAULT '[]'::jsonb,
    repro_steps text NOT NULL DEFAULT '',
    probe_paths jsonb NOT NULL DEFAULT '[]'::jsonb,
    continue_action text NOT NULL DEFAULT '',
    continue_comment_id uuid,
    log_dump text NOT NULL DEFAULT '',
    ingest_token text NOT NULL,
    work_dir_hint text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (id)
);
