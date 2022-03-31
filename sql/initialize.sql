CREATE TABLE kimmunity (
    schema_version INT NOT NULL PRIMARY KEY
);
INSERT INTO kimmunity(schema_version) VALUES(1);

CREATE TABLE users (
    username TEXT NOT NULL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    pwhash TEXT,
    profile_picture BOOL NOT NULL DEFAULT FALSE,
    name TEXT,
    CHECK(LOWER(username) NOT IN ('admin', 'administrator', 'root', 'owner', 'kim'))
);
CREATE UNIQUE INDEX idx_unique_email ON users(email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX idx_tsvector_username ON users(to_tsvector('english', username));
CREATE INDEX idx_tsvector_name ON users(to_tsvector('english', name));

CREATE VIEW view_users AS SELECT
    username,
    COALESCE(name, username) name,
    COALESCE(to_tsvector('english', name), to_tsvector('english', username)) tsv
FROM users;

CREATE TABLE sessions (
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    session_id TEXT NOT NULL PRIMARY KEY,
    expires BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM (NOW() + '1 day'::INTERVAL) AT TIME ZONE 'UTC')*1000)::BIGINT
);
CREATE VIEW valid_sessions AS SELECT
    username,
    session_id
FROM sessions WHERE expires > EXTRACT(epoch FROM NOW())*1000;

CREATE TABLE account_keys (
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    key TEXT NOT NULL UNIQUE,
    expires BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM (NOW() + '1 hour'::INTERVAL) AT TIME ZONE 'UTC')*1000)::BIGINT
);
CREATE VIEW valid_account_keys AS SELECT * 
FROM account_keys
WHERE expires > (EXTRACT(epoch FROM NOW() AT TIME ZONE 'UTC')*1000)::BIGINT;

CREATE TABLE posts (
    id TEXT NOT NULL PRIMARY KEY,
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    content TEXT NOT NULL,
    posted BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM NOW() AT TIME ZONE 'UTC')*1000)::BIGINT
);

CREATE TABLE images (
    post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    filename TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE follows (
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    follow TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    since BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM NOW() AT TIME ZONE 'UTC')*1000)::BIGINT
    CHECK(username <> follow),
    PRIMARY KEY(username, follow)
);

CREATE TABLE post_likes (
    post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    tstamp BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM NOW() AT TIME ZONE 'UTC')*1000)::BIGINT,
    PRIMARY KEY(post_id, username)
);

CREATE TABLE post_comments (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    content TEXT NOT NULL,
    tstamp BIGINT NOT NULL DEFAULT (EXTRACT(epoch FROM NOW() AT TIME ZONE 'UTC')*1000)::BIGINT
);
CREATE VIEW view_post_comments AS SELECT
    pc.id,
    pc.post_id,
    pc.username comment_by_username,
    u.name comment_by_name,
    pc.content,
    pc.tstamp
FROM post_comments pc
LEFT JOIN users u ON u.username = pc.username;

CREATE VIEW view_events AS SELECT * FROM
    (SELECT
        p.username event_for,
        'like' event_type,
        pl.post_id,
        pl.username event_by_username,
        COALESCE(u.name, u.username) event_by_name,
        pl.tstamp
    FROM post_likes pl
    LEFT JOIN posts p ON p.id = pl.post_id
    LEFT JOIN users u ON u.username = pl.username
    UNION ALL SELECT
        p.username event_for,
        'comment' event_type,
        pc.post_id,
        pc.username event_by_username,
        COALESCE(u.name, u.username) event_by_name,
        pc.tstamp
    FROM post_comments pc
    LEFT JOIN posts p ON p.id = pc.post_id
    LEFT JOIN users u ON u.username = pc.username
    UNION ALL SELECT
        f.follow event_for,
        'follow',
        NULL post_id,
        f.username event_by_username,
        COALESCE(u.name, u.username) event_by_name,
        f.since tstamp
    FROM follows f
    LEFT JOIN users u ON u.username = f.username) a
WHERE event_for <> event_by_username;

CREATE VIEW view_posts AS SELECT
    f.username,
    f.follow post_by_username,
    COALESCE(u.name, u.username) post_by_name,
    p.content,
    p.id post_id,
    p.posted
FROM follows f
LEFT JOIN users u ON u.username = f.follow
LEFT JOIN posts p ON p.username = f.follow
UNION ALL SELECT
    p.username,
    p.username post_by_username,
    COALESCE(u.name, u.username) post_by_username,
    p.content,
    p.id post_id,
    p.posted
FROM posts p
LEFT JOIN users u ON u.username = p.username;

CREATE TABLE page_summaries (
    url TEXT NOT NULL PRIMARY KEY,
    title TEXT,
    description TEXT,
    image TEXT
);