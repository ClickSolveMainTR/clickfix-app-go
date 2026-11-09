-- Data-loss note: drops the documents shared with the assistant (they
-- expire after 7 days anyway).
DROP TABLE IF EXISTS assistant_documents;
