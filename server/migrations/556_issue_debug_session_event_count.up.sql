ALTER TABLE issue_debug_session
    ADD COLUMN IF NOT EXISTS event_count integer NOT NULL DEFAULT 0;
