package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"go.uber.org/zap/zaptest"

	"telesrv/internal/domain"
)

// denyAllPrivacyService refuses every chat_invite, the state a bot account had
// before bot invitability was special-cased (default allow_contacts blocks
// strangers).
type denyAllPrivacyService struct{}

func (denyAllPrivacyService) GetRules(context.Context, int64, domain.PrivacyKey) (domain.PrivacyRules, error) {
	return domain.PrivacyRules{}, nil
}

func (denyAllPrivacyService) SetRules(_ context.Context, ownerUserID int64, key domain.PrivacyKey, rules []domain.PrivacyRule) (domain.PrivacyRules, error) {
	return domain.PrivacyRules{OwnerUserID: ownerUserID, Key: key, Rules: rules}, nil
}

func (denyAllPrivacyService) AddAllowUser(_ context.Context, ownerUserID int64, key domain.PrivacyKey, _ int64) (domain.PrivacyRules, bool, error) {
	return domain.PrivacyRules{OwnerUserID: ownerUserID, Key: key}, false, nil
}

func (denyAllPrivacyService) CanSee(context.Context, int64, int64, domain.PrivacyKey) (bool, error) {
	return false, nil
}

// TestFilterChatInvitePrivacyAlwaysAllowsBots pins the group-add behaviour: a
// bot must be invitable by a stranger even when its chat_invite privacy denies
// the inviter, while a human target with the same rules stays blocked.
func TestFilterChatInvitePrivacyAlwaysAllowsBots(t *testing.T) {
	ctx := context.Background()
	users := mapUsersService{users: map[int64]domain.User{
		1: {ID: 1, FirstName: "Human"},
		2: {ID: 2, FirstName: "Bot", Bot: true},
	}}
	r := New(Config{}, Deps{Users: users, Privacy: denyAllPrivacyService{}}, zaptest.NewLogger(t), clock.System)

	allowed, missing, err := r.filterChatInvitePrivacy(ctx, 1, []int64{2, 3})
	if err != nil {
		t.Fatalf("filter privacy: %v", err)
	}
	if len(allowed) != 1 || allowed[0] != 2 {
		t.Fatalf("allowed = %+v, want [2] (bot bypasses chat_invite privacy)", allowed)
	}
	if len(missing) != 1 || missing[0].UserID != 3 {
		t.Fatalf("missing = %+v, want [3] (unknown user stays blocked)", missing)
	}
}
