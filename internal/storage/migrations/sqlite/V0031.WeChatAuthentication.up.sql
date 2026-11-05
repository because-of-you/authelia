CREATE TABLE wechat_users (
    id INTEGER PRIMARY KEY,
    username VARCHAR(191) COLLATE BINARY NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    email VARCHAR(320),
    enabled INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE wechat_identities (
    user_id INTEGER NOT NULL REFERENCES wechat_users(id) ON DELETE CASCADE,
    appid VARCHAR(64) COLLATE BINARY NOT NULL,
    openid VARCHAR(128) COLLATE BINARY NOT NULL,
    unionid VARCHAR(128),
    PRIMARY KEY (appid, openid)
);
CREATE INDEX wechat_identities_user_id ON wechat_identities(user_id);
CREATE TABLE wechat_groups (id INTEGER PRIMARY KEY, name VARCHAR(191) COLLATE BINARY NOT NULL UNIQUE);
CREATE TABLE wechat_user_groups (
    user_id INTEGER NOT NULL REFERENCES wechat_users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL REFERENCES wechat_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);
CREATE TABLE wechat_miniapp_sessions (
    token_hash CHAR(64) COLLATE BINARY NOT NULL PRIMARY KEY,
    username VARCHAR(255) COLLATE BINARY NOT NULL,
    appid VARCHAR(64) COLLATE BINARY NOT NULL,
    openid VARCHAR(128) COLLATE BINARY NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX wechat_miniapp_sessions_expiry ON wechat_miniapp_sessions(expires_at);
CREATE TABLE wechat_qr_challenges (
    transaction_hash CHAR(64) COLLATE BINARY NOT NULL PRIMARY KEY,
    browser_hash CHAR(64) COLLATE BINARY NOT NULL,
    target_url VARCHAR(2048) NOT NULL, target_domain VARCHAR(255) NOT NULL,
    request_method VARCHAR(16) NOT NULL, flow_id VARCHAR(128) NOT NULL,
    flow VARCHAR(128) NOT NULL, subflow VARCHAR(128) NOT NULL, user_code VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL, approved_username VARCHAR(255) COLLATE BINARY,
    approved_appid VARCHAR(64) COLLATE BINARY, approved_openid VARCHAR(128) COLLATE BINARY,
    expires_at BIGINT NOT NULL, qr_image BLOB NOT NULL
);
CREATE INDEX wechat_qr_challenges_expiry ON wechat_qr_challenges(expires_at);
