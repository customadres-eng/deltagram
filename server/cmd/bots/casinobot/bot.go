package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iamxvbaba/td/tg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Bot is the DeltaGram casino assistant. It speaks Russian in chat while the
// Mini App is a Russian-UI casino.
type Bot struct {
	api          *tg.Client
	log          *zap.Logger
	webAppURL    string
	chipsPerStar int64

	pool *pgxpool.Pool

	self atomic.Pointer[tg.User]
	mu   sync.RWMutex
	// users caches the access hashes needed to answer a private chat.
	users map[int64]*tg.User
}

func newBot(log *zap.Logger, webAppURL string, chipsPerStar int64) *Bot {
	if chipsPerStar <= 0 {
		chipsPerStar = 100
	}
	return &Bot{
		log:          log,
		webAppURL:    webAppURL,
		chipsPerStar: chipsPerStar,
		users:        map[int64]*tg.User{},
	}
}

func (b *Bot) openStore(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return err
	}
	b.pool = pool
	return nil
}

func (b *Bot) closeStore() {
	if b.pool != nil {
		b.pool.Close()
	}
}

func (b *Bot) hasStore() bool { return b.pool != nil }

func (b *Bot) setSelf(u *tg.User) {
	if u == nil {
		return
	}
	b.self.Store(u)
	b.cacheUser(u)
}

func (b *Bot) selfUser() *tg.User { return b.self.Load() }

func (b *Bot) cacheUser(u *tg.User) {
	if u == nil || u.ID == 0 {
		return
	}
	b.mu.Lock()
	b.users[u.ID] = u
	b.mu.Unlock()
}

func (b *Bot) cachedUser(id int64) *tg.User {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.users[id]
}

// handle is the tg update handler.
func (b *Bot) handle(ctx context.Context, u tg.UpdatesClass) error {
	updates, users, _ := unpackUpdates(u)
	for _, uc := range users {
		if usr, ok := uc.(*tg.User); ok {
			b.cacheUser(usr)
		}
	}
	for _, up := range updates {
		var msg *tg.Message
		switch v := up.(type) {
		case *tg.UpdateNewMessage:
			msg, _ = v.Message.(*tg.Message)
		case *tg.UpdateNewChannelMessage:
			msg, _ = v.Message.(*tg.Message)
		}
		if msg != nil {
			b.handleMessage(ctx, msg)
		}
	}
	return nil
}

func unpackUpdates(u tg.UpdatesClass) (updates []tg.UpdateClass, users []tg.UserClass, chats []tg.ChatClass) {
	switch v := u.(type) {
	case *tg.Updates:
		return v.Updates, v.Users, v.Chats
	case *tg.UpdatesCombined:
		return v.Updates, v.Users, v.Chats
	case *tg.UpdateShort:
		return []tg.UpdateClass{v.Update}, nil, nil
	default:
		return nil, nil, nil
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tg.Message) {
	if msg == nil || msg.Out {
		return
	}
	text := strings.TrimSpace(msg.Message)
	if text == "" {
		return
	}
	fromID := peerUserID(msg.FromID)
	if fromID == 0 {
		return
	}
	if self := b.selfUser(); self != nil && fromID == self.ID {
		return
	}
	kind, chatID := "", int64(0)
	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		kind, chatID = "user", p.UserID
	case *tg.PeerChat:
		kind, chatID = "chat", p.ChatID
	case *tg.PeerChannel:
		kind, chatID = "channel", p.ChannelID
	}
	// The casino bot lives in private chats: group replies would need a
	// channel peer and would spam unrelated conversations.
	if kind != "user" || chatID == 0 {
		return
	}

	cmd, _, isCommand := parseCommand(text, b.selfUser())
	if !isCommand {
		b.send(ctx, chatID, startText(), b.openMarkup())
		return
	}

	switch normalizeCommand(cmd) {
	case "start":
		b.send(ctx, chatID, startText(), b.openMarkup())
	case "help":
		b.send(ctx, chatID, helpText(), b.openMarkup())
	case "balance":
		b.send(ctx, chatID, b.balanceText(ctx, fromID), b.openMarkup())
	default:
		b.send(ctx, chatID, "Неизвестная команда.\n\n"+startText(), b.openMarkup())
	}
}

// parseCommand splits "/start@bot arg" into its normalized command name. It
// accepts the /, ! and + prefixes like the rest of the DeltaGram bots.
func parseCommand(text string, self *tg.User) (cmd, rest string, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}
	switch text[0] {
	case '/', '+', '!':
	default:
		return "", "", false
	}
	cut := strings.IndexAny(text, " \t\n")
	head := text
	if cut >= 0 {
		head = text[:cut]
		rest = strings.TrimSpace(text[cut+1:])
	}
	head = head[1:]
	if at := strings.IndexByte(head, '@'); at >= 0 {
		mention := head[at+1:]
		if self == nil || !strings.EqualFold(mention, self.Username) {
			return "", "", false
		}
		head = head[:at]
	}
	head = strings.ToLower(strings.TrimSpace(head))
	if head == "" {
		return "", "", false
	}
	return head, rest, true
}

func normalizeCommand(cmd string) string {
	switch strings.ToLower(strings.TrimSpace(cmd)) {
	case "start", "старт", "почати":
		return "start"
	case "help", "правила", "допомога", "помощь":
		return "help"
	case "balance", "баланс", "гроші":
		return "balance"
	}
	return cmd
}

// openMarkup is the single inline web-view button that launches the Mini App.
// The URL points at the public HTTPS origin so DeltaGram can sign the launch
// parameters into it.
func (b *Bot) openMarkup() tg.ReplyMarkupClass {
	return &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{
			{
				Text: "🎰 Открыть казино",
				Type: &tg.InlineButtonTypeWebView{URL: b.webAppURL},
			},
		}},
	}}
}

func (b *Bot) send(ctx context.Context, userID int64, text string, markup tg.ReplyMarkupClass) error {
	peer := b.inputPeer(ctx, userID)
	if peer == nil {
		return errors.New("no input peer")
	}
	_, err := b.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     text,
		RandomID:    randomID(),
		ReplyMarkup: markup,
	})
	if err != nil {
		b.log.Warn("send failed", zap.Int64("user", userID), zap.Error(err))
	}
	return err
}

func (b *Bot) inputPeer(ctx context.Context, userID int64) tg.InputPeerClass {
	hash := int64(0)
	if u := b.cachedUser(userID); u != nil {
		hash = u.AccessHash
	} else if list, err := b.api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{UserID: userID}}); err == nil {
		for _, uc := range list {
			if u, ok := uc.(*tg.User); ok && u.ID == userID {
				b.cacheUser(u)
				hash = u.AccessHash
				break
			}
		}
	}
	return &tg.InputPeerUser{UserID: userID, AccessHash: hash}
}

// balanceText reads the player's chip balance straight from the casino schema.
// A player who never opened the table has no row yet, which is not an error.
func (b *Bot) balanceText(ctx context.Context, userID int64) string {
	if b.pool == nil {
		return "Баланс сейчас недоступен — откройте казино, чтобы увидеть его."
	}
	queryCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var chips int64
	err := b.pool.QueryRow(queryCtx, `SELECT balance FROM casino_accounts WHERE user_id = $1`, userID).Scan(&chips)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		b.log.Warn("read balance", zap.Int64("user", userID), zap.Error(err))
		return "Не удалось прочитать баланс. Попробуйте ещё раз через минуту."
	}
	stars := float64(chips) / float64(b.chipsPerStar)
	return fmt.Sprintf("Ваш баланс\n\nФишки: %s\nЗвёзды: %s\n\n1 ⭐ = %d фишек. Вывод — целыми звёздами.",
		formatInt(chips), formatStars(stars), b.chipsPerStar)
}

func startText() string {
	return "Delta Casino — игры на звёзды.\n\n" +
		"Crash, Mines, Upgrade и Slots с отдачей 95%. Каждый раунд можно проверить: сервер раскрывает свой ключ после игры.\n\n" +
		"Пополнение и вывод — звёздами. В Upgrade можно выиграть настоящий NFT.\n\n" +
		"Нажмите кнопку ниже, чтобы открыть казино."
}

func helpText() string {
	return "Как это работает\n\n" +
		"• Пополните баланс звёздами в разделе «Кошелёк».\n" +
		"• Выберите игру: Crash, Mines, Upgrade или Slots.\n" +
		"• Выигрыш зачисляется фишками, вывести можно целыми звёздами.\n" +
		"• Upgrade имеет режим NFT: приз — настоящий подарок из банка казино, иначе приходит фишками.\n" +
		"• Отдача всех игр — 95%. Минимальная ставка, пополнение и вывод — 100 звёзд.\n\n" +
		"Фишки и звёзды: 1 ⭐ = 100 фишек."
}

func formatInt(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out.WriteByte(' ')
		}
		out.WriteRune(r)
	}
	if neg {
		return "-" + out.String()
	}
	return out.String()
}

// formatStars prints a star amount with at most two decimals and a comma
// separator, trimming trailing zeros: 99.2 -> "99,2", 100 -> "100".
func formatStars(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return strings.ReplaceAll(s, ".", ",")
}

func peerUserID(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerUser:
		return v.UserID
	case *tg.PeerChat:
		return v.ChatID
	case *tg.PeerChannel:
		return v.ChannelID
	}
	return 0
}

func randomID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UnixNano()
	}
	id := int64(binary.LittleEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id
}
