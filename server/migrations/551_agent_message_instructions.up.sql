-- Optional standing text prepended onto every inbound turn this agent
-- receives (human comments, chat, steer, and agent-to-agent dispatch).
-- Distinct from agent.instructions, which is the identity/system prompt.
ALTER TABLE agent
    ADD COLUMN message_instructions TEXT NOT NULL DEFAULT '';

-- Two-step add matches agent_description_length: NOT VALID registers the
-- check without scanning, then VALIDATE scans under a weaker lock.
ALTER TABLE agent
    ADD CONSTRAINT agent_message_instructions_length
    CHECK (char_length(message_instructions) <= 4000) NOT VALID;

ALTER TABLE agent VALIDATE CONSTRAINT agent_message_instructions_length;
