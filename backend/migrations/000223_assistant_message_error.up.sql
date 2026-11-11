-- CF-207: a reply that failed (stream error) is told apart from one the user
-- stopped; the history shows a different label for each.
ALTER TABLE assistant_messages ADD COLUMN error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE assistant_messages ADD CONSTRAINT assistant_messages_error_code_chk CHECK (char_length(error_code) <= 40);
