-- Data-loss note: removes feedback, votes and published changelog content.
DROP TABLE changelog_entries, feedback_comments, feature_sources, feature_votes, feature_requests, feedback_settings;
