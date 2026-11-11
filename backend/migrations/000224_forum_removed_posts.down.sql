-- Data-loss note: posts taken down with CF-182 become `rejected`, and who
-- removed them, when and why is lost.
DROP INDEX IF EXISTS idx_forum_posts_removed;
ALTER TABLE forum_posts DROP CONSTRAINT forum_posts_removed_check;
UPDATE forum_posts SET status='rejected' WHERE status='removed';
ALTER TABLE forum_posts DROP COLUMN removed_at, DROP COLUMN removed_by_user_id, DROP COLUMN removed_reason;
ALTER TABLE forum_posts DROP CONSTRAINT forum_posts_status_check;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_status_check CHECK(status IN ('pending','published','rejected'));
