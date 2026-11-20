# Contributing

- Fork, branch, PR to `main`.
- Keep changes focused; one feature/fix per PR.
- Do not commit secrets (`.env`, tokens, keystore, `*.pem`).
- Android: run `./gradlew :TMessagesProj_App:assembleAfatDebug`.
- Server: `cd server && go build ./...`.
- Web: `cd server/cmd/telesrv-admin/web && npm ci && npm run build`.
- DeltaMent web: `cd server/cmd/deltament/web && npm ci && npm run build`.
- CI must be green before review.
- Maintainer: `@spivakkrytoi`.
