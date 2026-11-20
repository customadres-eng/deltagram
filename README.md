# DeltaGram

Monorepo for the DeltaGram ecosystem:

- `android/` — DeltaGram Android client (Telegram Android 13.1.2 / layer 232, `afat` flavor).
- `server/` — DeltaGram backend: Telegram-compatible server, admin panel, DeltaMent,
  casino, bots, migrations and Docker deployment.
- `docs/` — architecture, card/TON logic, build and contribution notes.

## Quick start: Android

```bash
git clone --recursive https://github.com/spivakkrytoi/deltagram.git
cd deltagram/android
./gradlew :TMessagesProj_App:assembleAfatDebug
```

The release build needs `TMessagesProj/config/release.keystore`. Do **not** commit it.
For CI, add the keystore as a base64 secret and let the workflow restore it.

## Quick start: server

```bash
cd server/deploy/docker
cp .env.example .env
# fill PostgreSQL, Redis, bot and admin tokens
podman-compose --env-file .env -f compose.yaml up -d --build
```

Services: `server` (MTProto), `admin` (web panel), `deltament` (market/wallet API),
`casino`, `casinobot`, `irisgram`, `mogbot`, plus PostgreSQL and Redis.

## Security

Never commit `.env`, bot tokens, admin tokens, database passwords or the release
keystore. Use GitHub Secrets / a private `.env` on the server.

## Contributing

1. Fork the repo.
2. Create a branch: `feat/my-feature` or `fix/my-bug`.
3. Open a Pull Request against `main`.
4. CI builds the server/web/Android debug APK.
5. A maintainer reviews and merges.

See `docs/CONTRIBUTING.md` and `docs/ARCHITECTURE.md`.
