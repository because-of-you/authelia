-- SPDX-FileCopyrightText: 2026 Authelia
--
-- SPDX-License-Identifier: Apache-2.0
-- Only identities inserted by an administrator can authenticate. Never use a
-- nickname or avatar as username. Preserve usernames once issued.

CREATE TABLE wechat_users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    display_name VARCHAR(255) CHARACTER SET utf8mb4 NOT NULL DEFAULT '',
    email VARCHAR(320) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY wechat_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- A stable Authelia account may be linked to identities from multiple appids.
CREATE TABLE wechat_identities (
    user_id BIGINT UNSIGNED NOT NULL,
    appid VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    openid VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    unionid VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
    PRIMARY KEY (appid, openid),
    KEY wechat_identities_user_id (user_id),
    CONSTRAINT wechat_identities_user_fk FOREIGN KEY (user_id) REFERENCES wechat_users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE wechat_groups (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY wechat_groups_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE wechat_user_groups (
    user_id BIGINT UNSIGNED NOT NULL,
    group_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, group_id),
    CONSTRAINT wechat_user_groups_user_fk FOREIGN KEY (user_id) REFERENCES wechat_users(id) ON DELETE CASCADE,
    CONSTRAINT wechat_user_groups_group_fk FOREIGN KEY (group_id) REFERENCES wechat_groups(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE wechat_miniapp_sessions (
  token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  username VARCHAR(255) NOT NULL,
  appid VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  openid VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  expires_at BIGINT NOT NULL,
  INDEX wechat_miniapp_sessions_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE wechat_qr_challenges (
  transaction_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  browser_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  target_url VARCHAR(2048) NOT NULL,
  target_domain VARCHAR(255) NOT NULL,
  request_method VARCHAR(16) NOT NULL,
  flow_id VARCHAR(128) NOT NULL,
  flow VARCHAR(128) NOT NULL,
  subflow VARCHAR(128) NOT NULL,
  user_code VARCHAR(128) NOT NULL,
  status VARCHAR(16) NOT NULL,
  approved_username VARCHAR(255),
  approved_appid VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin,
  approved_openid VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin,
  expires_at BIGINT NOT NULL,
  qr_image MEDIUMBLOB NOT NULL,
  INDEX wechat_qr_challenges_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE wechat_profiles (
    user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    nickname VARCHAR(120) NOT NULL,
    avatar MEDIUMBLOB NOT NULL,
    CONSTRAINT wechat_profiles_user_fk FOREIGN KEY (user_id) REFERENCES wechat_users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE UNIQUE INDEX wechat_users_email_unique ON wechat_users (email);
