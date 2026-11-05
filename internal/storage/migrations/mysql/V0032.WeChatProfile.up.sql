CREATE TABLE wechat_profiles (
    user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    nickname VARCHAR(120) NOT NULL,
    avatar MEDIUMBLOB NOT NULL,
    CONSTRAINT wechat_profiles_user_fk FOREIGN KEY (user_id) REFERENCES wechat_users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
