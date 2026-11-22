package postgres

import (
	"context"
	"fmt"
)

// AvatarFrames returns the DeltaGram cosmetic avatar frames granted to users.
//
// Deltament and the Telegram server share one database, so the registry reads
// the authoritative users.avatar_frame column directly instead of asking the
// admin API. Only rows that actually carry a frame are read thanks to the
// partial index created with the column.
func (db *DB) AvatarFrames(ctx context.Context) (map[int64]string, error) {
	rows, err := db.pool.Query(ctx, `
SELECT id, avatar_frame
FROM users
WHERE avatar_frame IN ('blue', 'fire', 'green', 'holographic_glass_ring', 'liquid_chrome_ring')
  AND deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list avatar frames: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]string)
	for rows.Next() {
		var (
			id    int64
			frame string
		)
		if err := rows.Scan(&id, &frame); err != nil {
			return nil, fmt.Errorf("scan avatar frame: %w", err)
		}
		out[id] = frame
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate avatar frames: %w", err)
	}
	return out, nil
}
