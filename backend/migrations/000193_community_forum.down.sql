-- Data-loss note: removes community posts, votes, subscriptions and notification history.
DROP TABLE forum_notifications, forum_subscriptions, forum_votes, forum_posts, forum_categories;
