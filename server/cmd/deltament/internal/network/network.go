package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"telesrv/cmd/deltament/internal/domain"
)

// поиск юзера на приватке по номеру
type Directory interface {
	ResolvePhone(ctx context.Context, phone string) (int64, error)
}

// отправка кода входа юзеру на приватке
type Messenger interface {
	SendCode(ctx context.Context, userID int64, text string) error
}

// выдача купленного на приватке
type Assets interface {
	GrantUsername(ctx context.Context, op Op, userID int64, username string) error
	GrantNumber(ctx context.Context, op Op, userID int64, phone string) error
	GrantGift(ctx context.Context, op Op, userID int64, giftID int64) error
	GrantStars(ctx context.Context, op Op, userID int64, amount int64) error
}

// списание звёзд с юзера на приватке
type Wallet interface {
	DebitStars(ctx context.Context, op Op, userID int64, amount int64) error
}

// ChannelAsset is one channel/supergroup created by the marketplace user.
type ChannelAsset struct {
	ID       int64
	Title    string
	Username string
}

// UsernameAsset is one collectible username held by the marketplace user.
type UsernameAsset struct {
	ID       int64
	Username string
}

// Inventory exposes the assets a user can put up for sale and the delivery
// operations the auction settler needs. It stays optional so simpler adapters
// (the generic webhook kind) keep working unchanged.
type Inventory interface {
	OwnedChannels(ctx context.Context, userID int64) ([]ChannelAsset, error)
	OwnedUsernames(ctx context.Context, userID int64) ([]UsernameAsset, error)
	DeleteChannel(ctx context.Context, op Op, channelID int64) error
	TransferUsername(ctx context.Context, op Op, username string, userID int64) error
}

// UsernameEscrow moves a seller's username into the operator vault when a lot
// settles. The marketplace then holds the asset until the winner binds it to a
// DeltaGram account, so a sold name can never stay in the seller's hands and a
// failed bind never costs the buyer the asset.
type UsernameEscrow interface {
	MintUsernameToVault(ctx context.Context, op Op, username string) error
	RevokeUsername(ctx context.Context, op Op, username string) error
}

type Pinger interface {
	Ping(ctx context.Context) error
}

// всё что умеет привязанный сервер
type Network interface {
	Directory
	Messenger
	Assets
	Wallet
	Pinger
}

// ключ идемпотентности и причина, чтобы повтор не выдал дважды.
// At и Value попадают в метаданные коллекционного юзернейма: дата покупки
// должна быть одинаковой при повторе, иначе сервер увидит другой запрос под
// тем же command_id. Value — цена лота в сотих GRAM.
type Op struct {
	Key    string
	Reason string
	At     time.Time
	Value  domain.Cents
}

// конструктор адаптера по типу сервера
type Factory func(apiURL, secret string) (Network, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// новый тип приватки регистрируется тут, сервисы трогать не надо
func Register(kind string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[kind] = f
}

func Kinds() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for k := range factories {
		out = append(out, k)
	}
	return out
}

func Build(kind, apiURL, secret string) (Network, error) {
	mu.RLock()
	f, ok := factories[kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: unknown server kind %q", domain.ErrInvalid, kind)
	}
	return f(apiURL, secret)
}
