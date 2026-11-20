# Deltament

Deltament — маркет для DeltaGram: юзернеймы, анонимные номера +888 и
коллекционные подарки. Вход и витрина всегда привязаны к одному серверу
DeltaGram; выбор приватки в интерфейсе убран. Пользователи входят по своему
номеру (обычному или анонимному +888) и покупают активы прямо на аккаунт.

Этот каталог — полноценный перенос
[Fragment-for-gramsrv](https://github.com/neptun42/Fragment-for-gramsrv) в
репозиторий gramsrv с ребрендингом под Deltament.

## Что внутри

```
cmd/deltament/
  main.go                 сборка зависимостей, HTTP-сервер
  bootstrap.go            servers.json + автопривязка DeltaGram
  static.go               встроенный собранный фронт (SPA fallback)
  web/                    React 19 + TypeScript + Vite + Tailwind
  internal/
    domain/               сущности, деньги в сотих GRAM, доменные ошибки
    network/              интерфейс приватки и реестр адаптеров
      gramsrv/            Admin API DeltaGram / telesrv
      webhook/            подписанные webhook-запросы для других движков
    service/              вход, маркет, аукционы, кошелёк, сервера
    store/postgres/       pgx, транзакции, миграции
    httpapi/              JSON API и авторизация по Bearer-токену
    secret/               AES-GCM для токенов приваток, хэши сессий
  CONNECT.md              как подключить приватку и контракт webhook
```

Фронт собирается в `cmd/deltament/web/dist` и вшивается в Go-бинарник через
`go:embed`. Dockerfile собирает фронт отдельным шагом, поэтому в образе
получается один самодостаточный бинарник.

## Быстрый старт

```bash
cd cmd/deltament/web
npm ci && npm run build
cd ../../..

export DELTAMENT_SECRET_KEY="$(openssl rand -hex 32)"
export DELTAMENT_DATABASE_URL="postgres://telesrv:password@127.0.0.1:5432/telesrv?sslmode=disable"
export DELTAMENT_DEFAULT_SERVER_TOKEN="$TELESRV_ADMIN_API_TOKEN"
go run ./cmd/deltament
```

Откройте `http://127.0.0.1:2700`. Если DeltaGram Admin API доступен на
`http://127.0.0.1:2599`, приватка привяжется автоматически при первом запуске
(с повторами, пока Admin API не поднимется). Операторский ключ `dlo_...`
выведется в лог один раз.

### Docker Compose

В `deploy/docker/.env` добавьте:

```env
DELTAMENT_SECRET_KEY=<случайная строка от 16 символов>
DELTAMENT_PORT=7000
DELTAMENT_DEFAULT_SERVER_API_URL=http://127.0.0.1:2599
DELTAMENT_DEFAULT_SERVER_STARS_PER_GRAM=0
```

`TELESRV_ADMIN_API_TOKEN` уже используется сервером DeltaGram, его же Deltament
берёт для автопривязки. В host-режиме сервис `deltament` ходит в Admin API по
адресу из `DELTAMENT_DEFAULT_SERVER_API_URL` (по умолчанию loopback). В bridge-
режиме `compose.bridge-network.yaml` переопределяет его на `http://server:2599`,
а базу — на `postgres:5432`; наружу публикуется только порт `DELTAMENT_PORT`.

## Переменные окружения

| переменная | по умолчанию | смысл |
| --- | --- | --- |
| `DELTAMENT_ADDR` | `127.0.0.1:2700` | адрес HTTP-сервера |
| `DELTAMENT_DATABASE_URL` | `DELTAMENT_POSTGRES_DSN`, затем `TELESRV_POSTGRES_DSN` | PostgreSQL DSN |
| `DELTAMENT_SECRET_KEY` | — | ключ AES-GCM, минимум 16 символов |
| `DELTAMENT_ADMIN_TOKEN` | пусто | Bearer для `/api/admin/*` |
| `DELTAMENT_AUTO_APPROVE` | `true` | сразу включать новые приватки |
| `DELTAMENT_WELCOME_BONUS` | `0` | бонус новым юзерам в GRAM |
| `DELTAMENT_CORS_ORIGIN` | пусто | Origin для отдельного фронта |
| `DELTAMENT_STATIC_DIR` | встроенный фронт | отдать фронт с диска |
| `DELTAMENT_SERVERS_FILE` | пусто | `servers.json` для привязки |
| `DELTAMENT_DEFAULT_SERVER_SLUG` | `deltagram` | slug автосервера |
| `DELTAMENT_PRIMARY_SERVER_SLUG` | `deltagram` | единственный сервер маркета |
| `DELTAMENT_DEFAULT_SERVER_NAME` | `DeltaGram` | имя на витрине |
| `DELTAMENT_DEFAULT_SERVER_API_URL` | `http://127.0.0.1:2599` | Admin API |
| `DELTAMENT_DEFAULT_SERVER_TOKEN` | `TELESRV_ADMIN_API_TOKEN` | токен Admin API |
| `DELTAMENT_DEFAULT_SERVER_STARS_PER_GRAM` | `0` | курс пополнения звёздами |
| `DELTAMENT_DEFAULT_SERVER_CONTACT` | пусто | контакт оператора |

## Как это работает

1. **Вход.** Юзер выбирает приватку и вводит номер. Deltament ищет аккаунт
   через Admin API (`resolve-by-phone`), генерирует 5-значный код и отправляет
   его личным сообщением через `broadcasts/create`. Код живёт 5 минут, максимум
   5 попыток, не больше 3 кодов за 10 минут. После проверки создаётся сессия,
   в localStorage кладётся Bearer-токен `dm_...`; в базе хранится только его
   SHA-256.
2. **Витрина.** Лоты выставляет оператор приватки через `/api/operator/lots`
   или `servers.json`. Виды: `username`, `number`, `gift`. Цена фиксированная
   или аукцион на 1 час — 30 дней. Пользователь также может выставить свой
   канал или коллекционный юзернейм по фиксированной цене либо на аукцион:
   инвентарь берётся из Admin API DeltaGram, поэтому продать чужой актив нельзя.
3. **Покупка.** Покупка идёт в транзакции с `SELECT ... FOR UPDATE` по лоту и
   пользователю. Баланс хранится в `int64` сотих GRAM, `CHECK (balance >= 0)`
   не даёт уйти в минус. Купленный актив попадает в инвентарь Deltament.
4. **Аукцион.** Минимальный шаг — 5% от текущей цены (не меньше 1 GRAM).
   Ставка сразу списывается; если её перебили, деньги возвращаются прошлому
   лидеру. Ставка в последние 5 минут продлевает аукцион ещё на 5 минут.
   Фоновый settler каждые 10 секунд закрывает истёкшие аукционы и отдаёт лот
   победителю. Если продавец — пользователь, канал удаляется через
   `POST /v1/channels/delete`, а освободившийся юзернейм минтится победителю
   как коллекционный; коллекционный юзернейм просто передаётся через
   `collectible-usernames/transfer`. Выручка зачисляется продавцу.
5. **Пополнение.** Юзер указывает количество GRAM, Deltament считает звёзды
   по курсу `stars_per_gram` и списывает их через `debit-stars`. Если курс `0`,
   пополнение выключено.
6. **Вывод.** Актив лежит в инвентаре, пока юзер не нажмёт «Вывести». Тогда
   Deltament вызывает `mint`/`transfer` для юзернейма или номера, `gifts/give`
   для подарка либо `grant-stars`. Баланс GRAM тоже можно вывести в звёзды:
   1 GRAM = `stars_per_gram` звёзд (по умолчанию 100), GRAM списывается с
   баланса маркета, звёзды приходят на аккаунт DeltaGram. У каждой операции
   стабильный `idempotency_key`, поэтому повтор не выдаст актив дважды.
7. **Продажа обратно.** Предмет возвращается на витрину по цене покупки, а
   стоимость зачисляется на баланс.
8. **Модерация.** Владелец Deltament может выключить приватку через
   `/api/admin/servers/{id}/status` (`Bearer $DELTAMENT_ADMIN_TOKEN`).

## Изоляция данных

Все таблицы маркета имеют префикс `deltament_market_`:
`servers`, `users`, `login_codes`, `sessions`, `history`, `lots`, `items`,
`bids`, `watchlist`. Таблица миграций — `deltament_market_schema_migrations`.
Старая реализация Deltament сохранена в `cmd/deltament-legacy` и использует
свои `deltament_*` таблицы, поэтому они не конфликтуют.

Токены приваток шифруются AES-GCM ключом `DELTAMENT_SECRET_KEY`. Ключи
операторов и сессии хранятся только как SHA-256-хэши.

## Сервер маркета

Публичная форма подключения приватки убрана. Deltament всегда использует
сервер из `DELTAMENT_PRIMARY_SERVER_SLUG` (по умолчанию `deltagram`), а
`/api/servers` отдаёт только его.

Технический контракт Admin API и webhook-подпись для других интеграций
остались в [CONNECT.md](CONNECT.md), но на работу публичного интерфейса они
не влияют.
