-- CF-182: a published post can be taken down. It moves to its own `removed`
-- status (not `rejected`: the moderation queue's decision stays final, and a
-- removed post can be published again), and the row keeps who removed it,
-- when and why. Public reads filter on status='published', so a removed
-- topic, its replies and a removed accepted reply disappear at once.
ALTER TABLE forum_posts DROP CONSTRAINT forum_posts_status_check;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_status_check CHECK(status IN ('pending','published','rejected','removed'));
ALTER TABLE forum_posts
 ADD COLUMN removed_at TIMESTAMPTZ,
 ADD COLUMN removed_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 ADD COLUMN removed_reason TEXT NOT NULL DEFAULT '' CHECK(length(removed_reason)<=500),
 ADD CONSTRAINT forum_posts_removed_check CHECK((status='removed') = (removed_at IS NOT NULL));
CREATE INDEX idx_forum_posts_removed ON forum_posts(organization_id,removed_at DESC) WHERE status='removed';
