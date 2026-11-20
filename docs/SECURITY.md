# Security

## Reporting

Report vulnerabilities privately to the maintainer (`@spivakkrytoi`) before
opening a public issue. Do not test money-moving endpoints on the production
server without written permission.

## Native DeltaGram API authentication

The card and `.ton` domain API used to trust a static token compiled into the
APK plus the `X-DeltaGram-User` header. That allowed anyone with the APK to act
as any Telegram user. It was replaced with a per-request proof:

1. The Android client signs a canonical request with the account's current
   **MTProto permanent auth key** (HMAC-SHA256).
2. The signature covers method, path, Unix timestamp, SHA-256 of the body and
   a random nonce.
3. DeltaMent sends the proof to the telesrv admin API.
4. telesrv resolves `auth_key_id -> user_id` from `authorizations`, verifies
   the HMAC with the stored 2048-bit auth key, rejects stale timestamps and
   replayed nonces, and returns the user id.
5. DeltaMent uses only that verified user id. `X-DeltaGram-User`,
   `X-DeltaGram-Token` and `user_id` query parameters are ignored.

Read-only public endpoints (`/api/cards/registry`, `/api/domains/listings`,
`/api/frames/registry`, `/api/banners/registry`) stay anonymous.

The client never contains a server secret. The static `deltagram-domains-v1`
token and its configuration default were removed.

## Secrets in this repository

Never commit:

- `.env`, `.env.bak*`, database/Redis passwords;
- bot tokens;
- admin API/UI tokens or session keys;
- `*.jks`, `*.keystore`, release signing keys;
- OTP webhook secrets;
- Huawei `agconnect-services.json` (`client_secret`);
- private keys other than third-party test fixtures.

`google-services.json` contains public Firebase Android API keys. They are not
server credentials, but they must be restricted in Google Cloud Console to the
DeltaGram package name and signing certificate SHA-1/SHA-256.

If a secret was ever committed, rotating it is mandatory. Deleting the file in
a later commit is not enough: Git history and forks keep old blobs.

## Deployment checklist

- Generate all secrets with `server/scripts/new-docker-env.sh` (or `.ps1`).
- Keep the real `.env` only on the server with mode `0600`.
- Rotate `TELESRV_ADMIN_API_TOKEN`, `TELESRV_ADMIN_UI_PASSWORD`,
  `TELESRV_OTP_WEBHOOK_SECRET`, `TELESRV_TURN_SECRET` and bot tokens after any
  suspected leak.
- Put the Android release keystore in GitHub Actions secrets, never in Git.
- Use branch protection and review every pull request.
