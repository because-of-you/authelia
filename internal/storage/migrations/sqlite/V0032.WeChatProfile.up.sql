CREATE TABLE wechat_profiles (
    user_id INTEGER PRIMARY KEY REFERENCES wechat_users(id) ON DELETE CASCADE,
    nickname VARCHAR(120) NOT NULL,
    avatar BLOB NOT NULL
);
