package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"telesrv/internal/domain"
)

// onlineSampleSystemIDs mirrors the admin console's online predicate: the
// built-in service accounts are not people and must not inflate the peak.
var onlineSampleSystemIDs = []int64{
	domain.OfficialSystemUserID,
	domain.BotFatherUserID,
	domain.StickersBotUserID,
	domain.ChatBotUserID,
}

// SampleOnlinePresence appends one row to presence_online_samples using the
// same five-minute last_seen_at window the console shows as "online now".
func SampleOnlinePresence(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
INSERT INTO presence_online_samples (online)
SELECT count(*)
FROM users
WHERE NOT is_bot
  AND deleted_at IS NULL
  AND id <> ALL($1::bigint[])
  AND last_seen_at > extract(epoch FROM now() - interval '5 minutes')::bigint`,
		onlineSampleSystemIDs)
	return err
}

// RunOnlinePresenceSampler records an online sample immediately and then on
// every tick until the context is cancelled. One row a minute is enough to make
// a daily peak meaningful while keeping the table tiny (1440 rows per day).
func RunOnlinePresenceSampler(ctx context.Context, pool *pgxpool.Pool, every time.Duration, log *zap.Logger) {
	if every <= 0 {
		every = time.Minute
	}
	sample := func() {
		if err := SampleOnlinePresence(ctx, pool); err != nil && ctx.Err() == nil {
			log.Warn("sample online presence", zap.Error(err))
		}
	}
	sample()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample()
		}
	}
}
