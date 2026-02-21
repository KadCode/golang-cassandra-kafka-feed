CREATE TABLE IF NOT EXISTS users (
    user_id uuid PRIMARY KEY,
    username text
);

CREATE TABLE IF NOT EXISTS users_by_username (
    username text PRIMARY KEY,
    user_id uuid
);

CREATE TABLE IF NOT EXISTS follows (
    user_id uuid,
    followee_id uuid,
    PRIMARY KEY (user_id, followee_id)
);

CREATE TABLE IF NOT EXISTS followers_by_followee (
    followee_id uuid,
    user_id uuid,
    PRIMARY KEY (followee_id, user_id)
);

CREATE TABLE IF NOT EXISTS posts (
    post_id uuid PRIMARY KEY,
    author_id uuid,
    body text,
    created_at timestamp
);

CREATE TABLE IF NOT EXISTS feed_by_user (
    user_id uuid,
    post_id uuid,
    author_id uuid,
    body text,
    created_at timestamp,
    PRIMARY KEY (user_id, created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
