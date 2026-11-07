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

Users may also enter an optional email address. Addresses are unique in the WeChat directory, ignoring case, but are not verified: the corresponding OIDC `email_verified` claim is false. Applications must not use an unverified email to link an existing account or make access decisions. WeChat does not provide the user's default email to the miniapp.

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

## Building the fork's container image

The `Build and publish image` GitHub Actions workflow builds `Dockerfile.image` and publishes to the repository's GitHub Container Registry package whenever a branch is pushed, or when started manually. It publishes `linux/amd64` and `linux/arm64` images under a branch tag and a `sha-<full-commit-sha>` tag. For this repository, the package is `ghcr.io/because-of-you/authelia`; the `codex/wechat-qr-auth` branch tag is `codex-wechat-qr-auth`.

The workflow uses the repository's `GITHUB_TOKEN` with `packages: write`; Actions must be enabled and allowed to publish packages. Check package visibility and configure image pull credentials where needed. Use a SHA tag or digest in deployment configuration so that the deployed image can be traced to an exact commit. Image publication does not update or deploy the Helm release.

## Merging upstream storage migrations

Authelia tracks a single global numeric schema version, not a separate version per filename or feature. This branch uses version 31 for WeChat. If upstream adds the same version, the migration loader detects the conflict and fails before applying migrations. Do not resolve the collision by discarding an upstream file or merely renaming the WeChat files.

Version 31 was consolidated for new databases. Earlier development databases which already applied the old versions 31 or 32 are not compatible with the consolidated migration and must be backed up and rebuilt, or migrated separately with a tested conversion. Do not manually edit migration history. Before deploying to an existing database, back it up and verify its schema version and migration path.

Longer term, a separately versioned WeChat extension migration system would avoid occupying upstream version numbers. This branch does not yet implement that conversion.
