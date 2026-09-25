ALTER TABLE agent DROP CONSTRAINT IF EXISTS agent_message_instructions_length;
ALTER TABLE agent DROP COLUMN IF EXISTS message_instructions;
