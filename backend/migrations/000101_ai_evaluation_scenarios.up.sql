-- Freeze replay conditions so business-hour and category policy changes can be
-- compared deterministically against the same golden scenario.
ALTER TABLE ai_evaluation_cases
 ADD COLUMN channel TEXT NOT NULL DEFAULT 'web' CHECK(length(channel) BETWEEN 1 AND 100),
 ADD COLUMN category TEXT NOT NULL DEFAULT '' CHECK(length(category)<=100),
 ADD COLUMN scenario_at TIMESTAMPTZ NOT NULL DEFAULT now();
