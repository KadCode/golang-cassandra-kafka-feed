CREATE KEYSPACE feedapp WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE feedapp;

CREATE TABLE users (
  user_id bigint PRIMARY KEY,
  username text
);

CREATE TABLE follows (
  user_id bigint,
  followee_id bigint,
  PRIMARY KEY (user_id, followee_id)
);

CREATE TABLE posts (
  post_id bigint PRIMARY KEY,
  author_id bigint,
  body text,
  created_at timestamp
);

CREATE TABLE feed_by_user (
  user_id bigint,
  post_id bigint,
  author_id bigint,
  body text,
  created_at timestamp,
  PRIMARY KEY (user_id, created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE followers_by_followee (
    followee_id bigint,
    user_id bigint,
    PRIMARY KEY (followee_id, user_id)
);