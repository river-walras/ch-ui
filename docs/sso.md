# Single Sign-On (OIDC SSO)

CH-UI supports OpenID Connect SSO against any compliant identity provider
(Okta, Microsoft Entra ID, Google Workspace, Keycloak, Auth0, …). SSO is a Pro
feature.

## How it works

OIDC authenticates the **person**. CH-UI does not receive a ClickHouse password
from the IdP, so queries run through a **per-connection ClickHouse service
account** that you configure. The person's identity (email) drives their CH-UI
role and is recorded in the audit trail; password login keeps working alongside
SSO.

```
Person ── OIDC ──▶ CH-UI   (identity, role, audit are per person)
                   CH-UI ── service account ──▶ ClickHouse
```

## 1. Register CH-UI with your IdP

Create an OAuth/OIDC application and set the redirect URI to:

```
https://ch-ui.yourcompany.com/api/auth/oidc/callback
```

Note the **issuer URL**, **client ID**, and **client secret**. If you want
role mapping, have the IdP include a `groups` claim in the ID token.

## 2. Configure CH-UI

Set these (env vars shown; `oidc_*` keys also work in the server config file):

```bash
OIDC_ISSUER_URL=https://accounts.google.com
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret
OIDC_REDIRECT_URL=https://ch-ui.yourcompany.com/api/auth/oidc/callback

# Optional:
OIDC_CONNECTION_ID=            # connection SSO uses (default: embedded connection)
OIDC_ALLOWED_DOMAINS=yourcompany.com   # restrict by email domain (comma-separated)
OIDC_GROUPS_CLAIM=groups       # ID-token claim holding group memberships
OIDC_ADMIN_GROUPS=ch-ui-admins # IdP groups → admin role (comma-separated)
OIDC_ANALYST_GROUPS=data-analysts  # IdP groups → analyst role
```

On startup you should see `OIDC SSO enabled`. A "Sign in with SSO" button then
appears on the login page. (If discovery fails, CH-UI logs the error and starts
with SSO disabled rather than refusing to boot.)

## 3. Set the ClickHouse service account

Configure the ClickHouse account that SSO sessions query through, on the target
connection (admin only):

```bash
curl -X PUT https://ch-ui.yourcompany.com/api/connections/<CONNECTION_ID>/sso-account \
  -H 'Content-Type: application/json' \
  --cookie 'chui_session=<admin session>' \
  -d '{"username": "ch_sso_reader", "password": "..."}'
```

The password is encrypted at rest with `APP_SECRET_KEY`. Until this is set, SSO
logins fail with a clear "service account not configured" message.

## Role mapping

| Condition | CH-UI role |
| --- | --- |
| Member of an `OIDC_ADMIN_GROUPS` group | `admin` |
| Member of an `OIDC_ANALYST_GROUPS` group | `analyst` |
| Otherwise | `viewer` |

## Security notes

- The flow uses `state` (CSRF) and `nonce` (replay) parameters, both verified on
  callback; the ID-token signature and audience are verified against the IdP's
  JWKS.
- Because all SSO users share one ClickHouse service account at the database
  layer, ClickHouse-native per-user grants do not apply to them — CH-UI's own
  RBAC (admin/analyst/viewer) is their access control. Pick the service
  account's ClickHouse grants accordingly (least privilege).
