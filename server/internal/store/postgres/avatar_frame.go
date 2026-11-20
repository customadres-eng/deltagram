package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"telesrv/internal/domain"
)

// SetAvatarFrame stores the DeltaGram cosmetic frame id for a user and returns
// the refreshed account with the new value populated.
//
// The column is intentionally not part of the sqlc-generated user model: frames
// never travel over MTProto, so the hot user read path keeps its exact column
// list and only the admin/registry paths touch this field.
func (s *UserStore) SetAvatarFrame(ctx context.Context, userID int64, frame domain.AvatarFrame) (domain.User, error) {
	if userID <= 0 {
		return domain.User{}, domain.ErrUserNotFound
	}
	tag, err := s.db.Exec(ctx, `
UPDATE users
SET avatar_frame = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL`, userID, string(frame))
	if err != nil {
		return domain.User{}, fmt.Errorf("update user avatar frame: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.User{}, domain.ErrUserNotFound
	}
	row, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("reload user after avatar frame update: %w", err)
	}
	u := userFromModel(row)
	u.AvatarFrame = frame
	return u, nil
}

// AvatarFrame returns the cosmetic frame currently granted to a user.
func (s *UserStore) AvatarFrame(ctx context.Context, userID int64) (domain.AvatarFrame, error) {
	var frame string
	err := s.db.QueryRow(ctx, `
SELECT avatar_frame FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&frame)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AvatarFrameNone, domain.ErrUserNotFound
		}
		return domain.AvatarFrameNone, fmt.Errorf("load user avatar frame: %w", err)
	}
	return domain.AvatarFrame(frame), nil
}
