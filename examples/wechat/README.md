# WeChat authentication

WeChat authentication uses the configured Authelia `storage` provider for the user directory, identity mappings, miniapp sessions, and QR transactions. Storage migrations create these tables. SQLite is suitable for local development; use the regular Authelia MySQL or PostgreSQL storage configuration when required. The user provider resolves accounts from those tables.

```yaml
authentication_backend:
  wechat_storage:
    enabled: true

wechat:
  app_id: 'wx-your-app-id'
  # Set AUTHELIA_WECHAT_APP_SECRET_FILE to a protected file containing the secret.
  qr_env_version: 'release' # Use develop or trial for matching pre-release miniapp versions.
```

Set the QR environment to `develop`, `trial`, or `release` to match the available miniapp version. Do not place AppSecret in the miniapp or source control. Configure `storage` as usual; no second MySQL connection is needed.

The authenticated miniapp profile stores the user's chosen nickname and avatar in the configured storage. These are user-supplied display data, not verified identity attributes. Avatars must be PNG or JPEG and at most 256 KiB; profile endpoints require the current miniapp session. Use HTTPS outside local development. Existing device-only profiles are not uploaded automatically; users can save them again.

A successfully verified WeChat identity is automatically registered without groups and is disabled by default. Disabled users can sign in to the miniapp and manage their own profile, but cannot approve a website login until an administrator enables the account. Existing disabled identities are not re-enabled automatically. The legacy `auto_register` setting is retained for compatibility and does not control registration. Nicknames and avatars are never used as account identifiers. For local SQLite, after the Authelia storage migrations have run, an example of manual provisioning is:

```sql
INSERT INTO wechat_users (username, display_name, enabled) VALUES ('user-001', 'Example User', 1);
INSERT INTO wechat_identities (user_id, appid, openid)
SELECT id, 'wx-your-app-id', 'verified-openid' FROM wechat_users WHERE username = 'user-001';
INSERT INTO wechat_groups (name) VALUES ('internal_users');
INSERT INTO wechat_user_groups (user_id, group_id)
SELECT u.id, g.id FROM wechat_users u CROSS JOIN wechat_groups g
WHERE u.username = 'user-001' AND g.name = 'internal_users';
```

The example requires a securely obtained and verified OpenID. Protect database access and do not put real identity values in source control. Disabling an account prevents website login approval, while access to the user's own miniapp profile remains available. Removing a binding invalidates existing sessions; the same identity can subsequently register a new disabled account. Previously issued OIDC tokens follow their configured lifetime and revocation policy.

The previous experimental `wechat_mysql` configuration and manually provisioned tables are not migrated automatically. Back up any existing data and plan an explicit migration before enabling this version against an existing deployment.

## Merging upstream storage migrations

Authelia tracks a single global numeric schema version, not a separate version per filename or feature. This branch currently uses versions 31 and 32 for WeChat. If upstream adds either version, keep both migration files during the merge: the migration loader detects the duplicate version and fails before applying migrations. Do not resolve the collision by discarding an upstream file or merely renaming the WeChat files.

Databases which already ran a WeChat migration have recorded those version numbers in Authelia's migration history. Changing a filename alone does not change that history and can cause an upstream migration to be skipped. Before merging or deploying such a change, back up affected databases, inventory which versions and WeChat tables each has, then design and test a one-time, provider-specific upgrade that preserves both upstream and WeChat data. Verify fresh databases and databases on each previously deployed version, including rollback where supported. Do not edit migration history manually without such a tested conversion.

Longer term, a separately versioned WeChat extension migration system would avoid occupying upstream version numbers, but it must include a tested conversion for databases already on versions 31 or 32. This branch does not yet implement that conversion.
