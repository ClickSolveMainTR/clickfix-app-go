-- Data-loss note: removes AI context history, imported source metadata, evaluation history and review queues. KB articles remain.
DROP TABLE ai_conversation_links;
DROP TABLE ai_evaluation_results;
DROP TABLE ai_evaluations;
DROP TABLE ai_evaluation_cases;
DROP TABLE knowledge_reviews;
DROP TABLE knowledge_translations;
DROP TABLE knowledge_suggestions;
DROP TABLE knowledge_source_documents;
DROP TABLE knowledge_sources;
DROP TABLE business_context_versions;
DROP TABLE business_context_sections;
