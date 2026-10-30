ALTER TABLE ai_runs DROP CONSTRAINT ai_runs_task_check;
ALTER TABLE ai_runs ADD CONSTRAINT ai_runs_task_check
    CHECK (task IN ('triage','reply','copilot','complex','embedding','transcribe'));
