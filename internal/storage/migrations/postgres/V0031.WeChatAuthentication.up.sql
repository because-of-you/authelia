CREATE TABLE wechat_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(191) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    email VARCHAR(320),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE wechat_identities (
    user_id BIGINT NOT NULL REFERENCES wechat_users(id) ON DELETE CASCADE,
    appid VARCHAR(64) NOT NULL,
    openid VARCHAR(128) NOT NULL,
    unionid VARCHAR(128),
    PRIMARY KEY (appid, openid)
);
CREATE INDEX wechat_identities_user_id ON wechat_identities(user_id);
CREATE TABLE wechat_groups (id BIGSERIAL PRIMARY KEY, name VARCHAR(191) NOT NULL UNIQUE);
CREATE TABLE wechat_user_groups (
    user_id BIGINT NOT NULL REFERENCES wechat_users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES wechat_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);
CREATE TABLE wechat_miniapp_sessions (
    token_hash CHAR(64) NOT NULL PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    appid VARCHAR(64) NOT NULL,
    openid VARCHAR(128) NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX wechat_miniapp_sessions_expiry ON wechat_miniapp_sessions(expires_at);
CREATE TABLE wechat_qr_challenges (
    transaction_hash CHAR(64) NOT NULL PRIMARY KEY,
    browser_hash CHAR(64) NOT NULL,
    target_url VARCHAR(2048) NOT NULL, target_domain VARCHAR(255) NOT NULL,
    request_method VARCHAR(16) NOT NULL, flow_id VARCHAR(128) NOT NULL,
    flow VARCHAR(128) NOT NULL, subflow VARCHAR(128) NOT NULL, user_code VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL, approved_username VARCHAR(255),
    approved_appid VARCHAR(64), approved_openid VARCHAR(128),
    expires_at BIGINT NOT NULL, qr_image BYTEA NOT NULL
);
CREATE INDEX wechat_qr_challenges_expiry ON wechat_qr_challenges(expires_at);
