package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"

	"telesrv/internal/botdata"
)

// Bot is the DeltaGram mog-battle bot.
type Bot struct {
	api        *tg.Client
	data       *botdata.Reader
	log        *zap.Logger
	self       atomic.Pointer[tg.User]
	cache      peerCache
	challenges *challengeStore
}

type peerCache struct {
	mu       sync.RWMutex
	users    map[int64]*tg.User
	channels map[int64]*tg.Channel
}

func newPeerCache() peerCache {
	return peerCache{users: map[int64]*tg.User{}, channels: map[int64]*tg.Channel{}}
}

func (c *peerCache) putUser(u *tg.User) {
	if u == nil || u.ID == 0 {
		return
	}
	c.mu.Lock()
	c.users[u.ID] = u
	c.mu.Unlock()
}

func (c *peerCache) getUser(id int64) *tg.User {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.users[id]
}

func (c *peerCache) putChannel(ch *tg.Channel) {
	if ch == nil || ch.ID == 0 {
		return
	}
	c.mu.Lock()
	c.channels[ch.ID] = ch
	c.mu.Unlock()
}

func (c *peerCache) getChannel(id int64) *tg.Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channels[id]
}

func (b *Bot) setSelf(u *tg.User) {
	if u == nil {
		return
	}
	b.self.Store(u)
	b.cache.putUser(u)
}

func (b *Bot) selfUser() *tg.User { return b.self.Load() }

type event struct {
	msg    *tg.Message
	chatID int64 // channel id, basic chat id, or user id in private
	kind   string
	fromID int64
	from   *tg.User
	cmd    string
	args   []string
}

// Handle is the tg update handler (called serially per update batch).
func (b *Bot) Handle(ctx context.Context, u tg.UpdatesClass) error {
	updates, users, chats := unpackUpdates(u)
	for _, uc := range users {
		if usr, ok := uc.(*tg.User); ok {
			b.cache.putUser(usr)
		}
	}
	for _, cc := range chats {
		if ch, ok := cc.(*tg.Channel); ok {
			b.cache.putChannel(ch)
		}
	}
	for _, up := range updates {
		switch v := up.(type) {
		case *tg.UpdateNewMessage:
			if msg, ok := v.Message.(*tg.Message); ok {
				b.handleIncoming(ctx, msg)
			}
		case *tg.UpdateNewChannelMessage:
			if msg, ok := v.Message.(*tg.Message); ok {
				b.handleIncoming(ctx, msg)
			}
		case *tg.UpdateBotCallbackQuery:
			b.handleCallback(ctx, v)
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

func (b *Bot) handleIncoming(ctx context.Context, msg *tg.Message) {
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
	kind := ""
	chatID := int64(0)
	switch p := msg.PeerID.(type) {
	case *tg.PeerChannel:
		chatID, kind = p.ChannelID, "channel"
	case *tg.PeerChat:
		chatID, kind = p.ChatID, "chat"
	case *tg.PeerUser:
		chatID, kind = p.UserID, "user"
	}
	if chatID == 0 {
		return
	}
	cmd, rest, ok := parseCommand(text, b.selfUser())
	if !ok {
		if kind == "user" {
			b.sendPrivate(ctx, chatID, helpText())
		}
		return
	}
	ev := &event{msg: msg, chatID: chatID, kind: kind, fromID: fromID, args: strings.Fields(rest)}
	ev.from = b.cache.getUser(fromID)
	if ev.from == nil {
		ev.from, _ = b.user(ctx, fromID)
	}
	switch normalizeCommand(cmd) {
	case "mog":
		b.cmdMog(ctx, ev)
	case "help", "start":
		b.sendEvent(ctx, ev, helpText(), nil)
	default:
		// Unknown commands stay silent: replying "unknown command" to every
		// unrelated /command in a busy group is just noise.
		return
	}
}

func (b *Bot) cmdMog(ctx context.Context, ev *event) {
	if len(ev.args) == 0 {
		b.sendEvent(ctx, ev, "Использование: /mog @username", nil)
		return
	}
	target := b.resolveUserToken(ctx, ev, ev.args[0])
	if target == nil {
		if uid := b.repliedUserID(ctx, ev); uid != 0 {
			target = b.userInChat(ctx, ev.chatID, uid)
		}
	}
	if target == nil {
		b.sendEvent(ctx, ev, "Не нашёл этого пользователя. Укажите @username или ответьте на сообщение.", nil)
		return
	}
	if target.ID == ev.fromID {
		b.sendEvent(ctx, ev, "Нельзя вызвать самого себя на мог-батл.", nil)
		return
	}
	if target.Bot {
		b.sendEvent(ctx, ev, "Боты не участвуют в мог-батлах.", nil)
		return
	}
	// Show the exact handle the challenger typed. Accounts that own collectible
	// usernames have an empty scalar Username and a Usernames vector, so the
	// first vector entry is not necessarily the handle used in the command.
	targetTag := userTag(target)
	if handle := strings.TrimSpace(ev.args[0]); strings.HasPrefix(handle, "@") && len(handle) > 1 {
		targetTag = handle
	}
	ch := b.challenges.create(ev.chatID, ev.kind, ev.fromID, target.ID, userTag(ev.from), targetTag)
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("✅ Принять", fmt.Sprintf("mog:accept:%d", ch.ID)),
			inlineButton("❌ Отклонить", fmt.Sprintf("mog:decline:%d", ch.ID)),
		}},
	}}
	text := fmt.Sprintf("⚔️ %s вызывает %s на мог-батл!\n\n%s, принимаешь вызов? У тебя 10 минут.",
		userTag(ev.from), targetTag, targetTag)
	b.sendEvent(ctx, ev, text, markup)
}

func (b *Bot) handleCallback(ctx context.Context, q *tg.UpdateBotCallbackQuery) {
	data := string(q.Data)
	const prefix = "mog:"
	if !strings.HasPrefix(data, prefix) {
		b.answerCallback(ctx, q.QueryID, "Кнопка устарела.", true)
		return
	}
	parts := strings.Split(strings.TrimPrefix(data, prefix), ":")
	if len(parts) != 2 {
		b.answerCallback(ctx, q.QueryID, "Некорректная кнопка.", true)
		return
	}
	ch, ok := b.challenges.get(parts[1])
	if !ok {
		b.answerCallback(ctx, q.QueryID, "Этот вызов уже истёк или был отменён.", true)
		return
	}
	if q.UserID != ch.TargetID {
		b.answerCallback(ctx, q.QueryID, "Этот вызов адресован другому пользователю.", true)
		return
	}
	b.challenges.remove(ch.ID)

	switch parts[0] {
	case "accept":
		b.answerCallback(ctx, q.QueryID, "Вызов принят!", false)
		b.sendBattleResult(ctx, ch)
	case "decline":
		b.answerCallback(ctx, q.QueryID, "Вызов отклонён.", false)
		b.sendTo(ctx, ch.ChatID, ch.Kind, fmt.Sprintf("❌ %s отклонил вызов. Мог-батл не состоялся.",
			ch.TargetTag), 0, nil)
	default:
		b.answerCallback(ctx, q.QueryID, "Неизвестное действие.", true)
	}
}

func (b *Bot) answerCallback(ctx context.Context, queryID int64, text string, alert bool) {
	req := &tg.MessagesSetBotCallbackAnswerRequest{QueryID: queryID, Message: text, Alert: alert}
	if _, err := b.api.MessagesSetBotCallbackAnswer(ctx, req); err != nil {
		b.log.Debug("answer callback failed", zap.Error(err))
	}
}

// ---- command parsing / targets ----

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
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	cmd = strings.ReplaceAll(cmd, "_", "-")
	switch cmd {
	case "мог", "могбатл", "mog", "mogbattle", "mog-battle":
		return "mog"
	case "помощь", "help", "команды", "старт", "start":
		return "help"
	}
	return cmd
}

func (b *Bot) resolveUserToken(ctx context.Context, ev *event, token string) *tg.User {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if strings.HasPrefix(token, "@") {
		return b.findUser(ctx, ev.chatID, token[1:])
	}
	if isDigits(token) {
		var id int64
		_, _ = fmt.Sscan(token, &id)
		return b.userInChat(ctx, ev.chatID, id)
	}
	if strings.HasPrefix(strings.ToLower(token), "id") && isDigits(token[2:]) {
		var id int64
		_, _ = fmt.Sscan(token[2:], &id)
		return b.userInChat(ctx, ev.chatID, id)
	}
	return b.findUser(ctx, ev.chatID, token)
}

func (b *Bot) findUser(ctx context.Context, chatID int64, query string) *tg.User {
	query = strings.TrimPrefix(strings.TrimSpace(query), "@")
	if query == "" {
		return nil
	}
	if isDigits(query) {
		var id int64
		_, _ = fmt.Sscan(query, &id)
		return b.userInChat(ctx, chatID, id)
	}
	if u, err := b.resolveUsername(ctx, query); err == nil && u != nil {
		return u
	}
	return b.searchParticipant(ctx, chatID, query)
}

func (b *Bot) userInChat(ctx context.Context, chatID, userID int64) *tg.User {
	if userID == 0 {
		return nil
	}
	if u := b.cache.getUser(userID); u != nil {
		return u
	}
	if u, err := b.user(ctx, userID); err == nil && u != nil {
		return u
	}
	if u := b.participantByID(ctx, chatID, userID); u != nil {
		return u
	}
	return &tg.User{ID: userID}
}

func (b *Bot) participantByID(ctx context.Context, chatID, userID int64) *tg.User {
	ch := b.channel(ctx, chatID)
	if ch == nil {
		return nil
	}
	res, err := b.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Filter:  &tg.ChannelParticipantsRecent{}, Offset: 0, Limit: 200,
	})
	if err != nil {
		return nil
	}
	data, ok := res.(*tg.ChannelsChannelParticipants)
	if !ok {
		return nil
	}
	for _, uc := range data.Users {
		if u, ok := uc.(*tg.User); ok {
			b.cache.putUser(u)
			if u.ID == userID {
				return u
			}
		}
	}
	return nil
}

func (b *Bot) searchParticipant(ctx context.Context, chatID int64, query string) *tg.User {
	ch := b.channel(ctx, chatID)
	if ch == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	res, err := b.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Filter:  &tg.ChannelParticipantsSearch{Q: query}, Offset: 0, Limit: 200,
	})
	if err != nil {
		return nil
	}
	data, ok := res.(*tg.ChannelsChannelParticipants)
	if !ok {
		return nil
	}
	var users []*tg.User
	for _, uc := range data.Users {
		if u, ok := uc.(*tg.User); ok {
			b.cache.putUser(u)
			users = append(users, u)
		}
	}
	return matchUser(users, query)
}

func matchUser(users []*tg.User, query string) *tg.User {
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, "@")))
	if q == "" {
		return nil
	}
	for _, u := range users {
		if u != nil && strings.EqualFold(u.Username, q) {
			return u
		}
	}
	for _, u := range users {
		if u != nil && strings.EqualFold(userFullName(u), q) {
			return u
		}
	}
	return nil
}

func (b *Bot) repliedUserID(_ context.Context, ev *event) int64 {
	if ev == nil || ev.msg == nil {
		return 0
	}
	hdr, ok := ev.msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || hdr.ReplyToMsgID <= 0 {
		return 0
	}
	return peerUserID(hdr.ReplyFrom.FromID)
}

// ---- DeltaGram API helpers ----

func (b *Bot) channel(ctx context.Context, id int64) *tg.Channel {
	if id == 0 {
		return nil
	}
	if ch := b.cache.getChannel(id); ch != nil {
		return ch
	}
	res, err := b.api.ChannelsGetChannels(ctx, []tg.InputChannelClass{&tg.InputChannel{ChannelID: id}})
	if err != nil {
		return nil
	}
	for _, cc := range res.GetChats() {
		if ch, ok := cc.(*tg.Channel); ok && ch.ID == id {
			b.cache.putChannel(ch)
			return ch
		}
	}
	return nil
}

func (b *Bot) user(ctx context.Context, id int64) (*tg.User, error) {
	if id == 0 {
		return nil, fmt.Errorf("no user")
	}
	if u := b.cache.getUser(id); u != nil {
		return u, nil
	}
	list, err := b.api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{UserID: id}})
	if err != nil {
		return nil, err
	}
	for _, uc := range list {
		if u, ok := uc.(*tg.User); ok && u.ID == id {
			b.cache.putUser(u)
			return u, nil
		}
	}
	return nil, fmt.Errorf("user %d not found", id)
}

func (b *Bot) resolveUsername(ctx context.Context, username string) (*tg.User, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, fmt.Errorf("empty username")
	}
	res, err := b.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, err
	}
	peerID := int64(0)
	if pu, ok := res.Peer.(*tg.PeerUser); ok {
		peerID = pu.UserID
	}
	var peerUser *tg.User
	for _, uc := range res.Users {
		u, ok := uc.(*tg.User)
		if !ok || u == nil {
			continue
		}
		b.cache.putUser(u)
		if userHasUsername(u, username) {
			return u, nil
		}
		// An account with a collectible username projects an empty scalar
		// Username, so the vector can be absent or the requested alias may not
		// be listed. contacts.resolveUsername still returns the authoritative
		// PeerUser; remember that object as the fallback.
		if peerID != 0 && u.ID == peerID {
			peerUser = u
		}
	}
	if peerUser != nil {
		return peerUser, nil
	}
	if peerID != 0 {
		if u := b.cache.getUser(peerID); u != nil {
			return u, nil
		}
		if u, err := b.user(ctx, peerID); err == nil && u != nil {
			return u, nil
		}
		return &tg.User{ID: peerID}, nil
	}
	return nil, fmt.Errorf("user @%s not found", username)
}

func userHasUsername(u *tg.User, query string) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(u.Username, query) {
		return true
	}
	if names, ok := u.GetUsernames(); ok {
		for _, item := range names {
			if item.Active && strings.EqualFold(item.Username, query) {
				return true
			}
		}
	}
	return false
}

// primaryUsername returns the handle to show for a user. The scalar column is
// empty for accounts that own collectible usernames; the first active entry of
// the vector is the best public stand-in there.
func primaryUsername(u *tg.User) string {
	if u == nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	if names, ok := u.GetUsernames(); ok {
		for _, item := range names {
			if item.Active && item.Username != "" {
				return item.Username
			}
		}
	}
	return ""
}

func (b *Bot) channelHash(ctx context.Context, id int64) int64 {
	if ch := b.channel(ctx, id); ch != nil {
		return ch.AccessHash
	}
	return 0
}

func (b *Bot) peerFor(ctx context.Context, kind string, chatID int64) tg.InputPeerClass {
	switch kind {
	case "channel":
		return &tg.InputPeerChannel{ChannelID: chatID, AccessHash: b.channelHash(ctx, chatID)}
	case "chat":
		return &tg.InputPeerChat{ChatID: chatID}
	case "user":
		var hash int64
		if u, err := b.user(ctx, chatID); err == nil && u != nil {
			hash = u.AccessHash
		}
		return &tg.InputPeerUser{UserID: chatID, AccessHash: hash}
	default:
		return nil
	}
}

func (b *Bot) send(ctx context.Context, peer tg.InputPeerClass, text string, replyTo int, markup tg.ReplyMarkupClass) (*tg.Message, error) {
	req := &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     text,
		RandomID:    randomID(),
		ReplyMarkup: markup,
	}
	if replyTo > 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyTo}
	}
	up, err := b.api.MessagesSendMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return firstMessage(up), nil
}

func (b *Bot) sendTo(ctx context.Context, chatID int64, kind, text string, replyTo int, markup tg.ReplyMarkupClass) {
	peer := b.peerFor(ctx, kind, chatID)
	if peer == nil {
		return
	}
	if _, err := b.send(ctx, peer, text, replyTo, markup); err != nil {
		b.log.Warn("send failed", zap.Int64("chat", chatID), zap.Error(err))
	}
}

func (b *Bot) sendEvent(ctx context.Context, ev *event, text string, markup tg.ReplyMarkupClass) {
	if ev == nil {
		return
	}
	replyTo := 0
	if ev.kind == "channel" {
		replyTo = ev.msg.ID
	}
	b.sendTo(ctx, ev.chatID, ev.kind, text, replyTo, markup)
}

func (b *Bot) sendPrivate(ctx context.Context, userID int64, text string) {
	b.sendTo(ctx, userID, "user", text, 0, nil)
}

func inlineButton(text, data string) tg.KeyboardInlineButton {
	return tg.KeyboardInlineButton{
		Text: text,
		Type: &tg.InlineButtonTypeCallback{Data: []byte(data)},
	}
}

func firstMessage(up tg.UpdatesClass) *tg.Message {
	check := func(update tg.UpdateClass) *tg.Message {
		switch v := update.(type) {
		case *tg.UpdateNewMessage:
			if m, ok := v.Message.(*tg.Message); ok {
				return m
			}
		case *tg.UpdateNewChannelMessage:
			if m, ok := v.Message.(*tg.Message); ok {
				return m
			}
		}
		return nil
	}
	switch v := up.(type) {
	case *tg.Updates:
		for _, u := range v.Updates {
			if m := check(u); m != nil {
				return m
			}
		}
	case *tg.UpdatesCombined:
		for _, u := range v.Updates {
			if m := check(u); m != nil {
				return m
			}
		}
	case *tg.UpdateShort:
		return check(v.Update)
	}
	return nil
}

func peerUserID(p tg.PeerClass) int64 {
	if u, ok := p.(*tg.PeerUser); ok {
		return u.UserID
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

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func userFullName(u *tg.User) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
}

func userLabel(u *tg.User) string {
	if u == nil {
		return "пользователь"
	}
	if name := userFullName(u); name != "" {
		return name
	}
	if username := primaryUsername(u); username != "" {
		return "@" + username
	}
	return fmt.Sprintf("id%d", u.ID)
}

func userTag(u *tg.User) string {
	if u == nil {
		return "пользователь"
	}
	if username := primaryUsername(u); username != "" {
		return "@" + username
	}
	return fmt.Sprintf("%s (id%d)", userLabel(u), u.ID)
}

func userTagByID(id int64, name string) string {
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("id%d", id)
	}
	return name
}

func helpText() string {
	return "⚔️ Мог-батл\n\n" +
		"Вызовите участника: /mog @username\n" +
		"Можно ответить на сообщение: /mog (ответом на сообщение).\n\n" +
		"Второй участник принимает или отклоняет вызов кнопкой в течение 10 минут.\n\n" +
		"После принятия я сравниваю только два показателя:\n" +
		"• баланс звёзд;\n" +
		"• количество подарков на чёрном фоне.\n\n" +
		"Обычные подарки в зачёт не идут. Победитель определяется по большинству из этих двух категорий; при равенстве — ничья. Я открыто показываю цифры, счёт и причину решения."
}
