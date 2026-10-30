-- Data-loss note (classification only): copilot records revert to the shared reply task label.
UPDATE ai_runs SET task = 'reply' WHERE task = 'copilot';
ALTER TABLE ai_runs DROP CONSTRAINT ai_runs_task_check;
ALTER TABLE ai_runs ADD CONSTRAINT ai_runs_task_check
    CHECK (task IN ('triage','reply','complex','embedding','transcribe'));
