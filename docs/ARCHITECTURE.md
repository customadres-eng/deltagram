# DeltaGram architecture

## Android client

- Base: Telegram Android 13.1.2, MTProto layer 232, flavor `afat`.
- Package `org.deltagram.android`; standalone `org.deltagram.android2`; debug `.beta`.
- Custom code lives mostly in `TMessagesProj/src/main/java/org/telegram/ui/Wallet/`,
  `org/telegram/messenger/Deltagram*.java`, `DeltagramFrames`, `Theme`.
- Cards are stock `WalletCardComponent` instances. The second card uses a gold
  palette via `setStickerStyle`.
- Public card data: `GET /api/cards/registry` (balances, colour, NFT, TON address).
- Transfers: `POST /api/cards/send`.
- `.ton` registry: `/api/domains/*` and `DeltamentDomainsActivity`.

## Server / DeltaMent

- One PostgreSQL database shared by the Telegram server and DeltaMent.
- `ton_balances` — off-chain TON ledger shown on the client card.
- `stars_balances` — Stars ledger.
- `deltament_balance_transactions` — wallet history.
- `deltament_ton_cards.address` — payout address (UQ/EQ or owned `.ton`).
- `deltament_card_prefs` — public card colour and NFT.
- `ton_domains` — `.ton` registry (buy, sell, 10% fee, owner check).
- Withdrawals create `deltament_ton_payouts`; the owner settles them manually.
- OTP: login codes are issued by the Telegram-compatible server and delivered to
  `grammystore`/`@deltagram_robot`; new numbers fall back to operator chats.

## Deployment

`server/deploy/docker/compose.yaml` builds every service from the same `server/`
tree. PostgreSQL/Redis use named volumes. See `docs/BUILD.md`.
