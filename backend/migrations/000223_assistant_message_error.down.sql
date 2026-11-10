-- Data-loss note: the failure code of interrupted replies is lost.
ALTER TABLE assistant_messages DROP CONSTRAINT assistant_messages_error_code_chk;
ALTER TABLE assistant_messages DROP COLUMN error_code;
