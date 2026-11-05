CREATE TABLE wechat_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES wechat_users(id) ON DELETE CASCADE,
    nickname VARCHAR(120) NOT NULL,
    avatar BYTEA NOT NULL
);
