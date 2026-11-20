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
	"unicode/utf8"

	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"

	"telesrv/internal/botdata"
)

// Bot is the DeltaGram moderation bot. It is an external MTProto client that
// logs in with the token issued by the DeltaGram Admin API.
type Bot struct {
	api   *tg.Client
	store *Store
	data  *botdata.Reader
	log   *zap.Logger
	self  atomic.Pointer[tg.User]
	cache peerCache
}

type peerCache struct {
	mu       sync.RWMutex
	users    map[int64]*tg.User
	channels map[int64]*tg.Channel
	messages map[messageKey]*tg.Message
}

type messageKey struct {
	chatID int64
	msgID  int
}

func newPeerCache() peerCache {
	return peerCache{
		users:    map[int64]*tg.User{},
		channels: map[int64]*tg.Channel{},
		messages: map[messageKey]*tg.Message{},
	}
}

func (c *peerCache) putMessage(chatID int64, m *tg.Message) {
	if m == nil || m.ID <= 0 || chatID == 0 {
		return
	}
	c.mu.Lock()
	c.messages[messageKey{chatID: chatID, msgID: m.ID}] = m
	c.mu.Unlock()
}

func (c *peerCache) getMessage(chatID int64, msgID int) *tg.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messages[messageKey{chatID: chatID, msgID: msgID}]
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

// Handle is the tg update handler. td calls it serially for every update batch.
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
				b.rememberMessage(msg)
				b.handleIncoming(ctx, msg)
			}
		case *tg.UpdateNewChannelMessage:
			if msg, ok := v.Message.(*tg.Message); ok {
				b.rememberMessage(msg)
				b.handleIncoming(ctx, msg)
			}
		case *tg.UpdateBotCallbackQuery:
			b.handleCallback(ctx, v)
		}
	}
	return nil
}

// rememberMessage keeps recent group messages in memory so a reply can be
// resolved even when channels.getMessages is not available to the bot.
func (b *Bot) rememberMessage(m *tg.Message) {
	if m == nil || m.ID <= 0 {
		return
	}
	chatID := int64(0)
	switch p := m.PeerID.(type) {
	case *tg.PeerChannel:
		chatID = p.ChannelID
	case *tg.PeerChat:
		chatID = p.ChatID
	}
	if chatID == 0 {
		return
	}
	b.cache.putMessage(chatID, m)
	if u := b.cache.getUser(peerUserID(m.FromID)); u != nil {
		b.cache.putUser(u)
	}
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

type event struct {
	msg    *tg.Message
	chatID int64 // positive channel id, or basic chat id, or user id in private
	kind   string
	fromID int64
	from   *tg.User
	cmd    string
	args   []string
	rest   string
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
	self := b.selfUser()
	if self != nil && fromID == self.ID {
		return
	}
	chatID := int64(0)
	kind := ""
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

	ev := &event{msg: msg, chatID: chatID, kind: kind, fromID: fromID}
	ev.from = b.cache.getUser(fromID)
	if ev.from == nil {
		ev.from, _ = b.user(ctx, fromID)
	}

	cmd, rest, ok := parseCommand(text, self)
	if !ok {
		if kind != "user" {
			return
		}
		b.sendEvent(ctx, ev, privateIntroText(self), nil)
		return
	}
	ev.cmd = normalizeCommand(cmd)
	ev.rest = rest
	ev.args = strings.Fields(rest)

	if kind == "user" {
		b.dispatchPrivate(ctx, ev)
		return
	}
	b.dispatch(ctx, ev)
}

func (b *Bot) handleCallback(ctx context.Context, q *tg.UpdateBotCallbackQuery) {
	data := string(q.Data)
	if strings.HasPrefix(data, "md:") {
		b.handleModeratorCallback(ctx, q, data)
		return
	}
	if !strings.HasPrefix(data, "st:") {
		b.answerCallback(ctx, q.QueryID, "Кнопка устарела.", true)
		return
	}
	chatID := int64(0)
	switch p := q.Peer.(type) {
	case *tg.PeerChannel:
		chatID = p.ChannelID
	case *tg.PeerChat:
		chatID = p.ChatID
	default:
		b.answerCallback(ctx, q.QueryID, "Эта панель работает только в группе.", true)
		return
	}
	if !b.isOwner(ctx, chatID, q.UserID) {
		b.answerCallback(ctx, q.QueryID, "Менять настройки может только владелец группы.", true)
		return
	}
	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		b.answerCallback(ctx, q.QueryID, "Некорректная кнопка.", true)
		return
	}
	field, value := parts[1], parts[2]

	switch field {
	case "close":
		b.answerCallback(ctx, q.QueryID, "Панель закрыта.", false)
		peer := b.peerFor(chatID)
		if peer != nil {
			_, _ = b.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:    peer,
				ID:      q.MsgID,
				Message: "⚙️ Панель настроек закрыта.",
			})
		}
		return
	case "mods":
		b.editPanel(ctx, chatID, q.MsgID, moderatorListText(b, ctx, chatID), moderatorListMarkup(b.store.Moderators(chatID)))
		b.answerCallback(ctx, q.QueryID, "", false)
		return
	case "reset":
		settings, err := b.store.ResetSettings(chatID)
		if err != nil {
			b.answerCallback(ctx, q.QueryID, "Не удалось сохранить настройки.", true)
			return
		}
		b.editSettingsPanel(ctx, chatID, q.MsgID, settings)
		b.answerCallback(ctx, q.QueryID, "Настройки сброшены.", false)
		return
	}

	settings, err := b.store.UpdateSettings(chatID, func(s *Settings) {
		b.applySettingButton(ctx, chatID, s, field, value)
	})
	if err != nil {
		b.answerCallback(ctx, q.QueryID, "Не удалось сохранить настройки.", true)
		return
	}
	b.editSettingsPanel(ctx, chatID, q.MsgID, settings)
	b.answerCallback(ctx, q.QueryID, "Сохранено.", false)
}

func (b *Bot) applySettingButton(ctx context.Context, chatID int64, s *Settings, field, value string) {
	switch field {
	case "limit":
		switch value {
		case "-":
			s.WarnLimit--
		case "+":
			s.WarnLimit++
		}
	case "mute":
		s.MuteSeconds = nextDuration(s.MuteSeconds, mutePresets)
	case "warntime":
		s.WarnExpirySeconds = nextDuration(s.WarnExpirySeconds, warnPresets)
	case "ban":
		s.BanSeconds = nextDuration(s.BanSeconds, banPresets)
	case "punish":
		switch normalizeWarnPunish(s.WarnPunish) {
		case "mute":
			s.WarnPunish = "ban"
		case "ban":
			s.WarnPunish = "kick"
		default:
			s.WarnPunish = "mute"
		}
	}
}

func (b *Bot) answerCallback(ctx context.Context, queryID int64, text string, alert bool) {
	req := &tg.MessagesSetBotCallbackAnswerRequest{QueryID: queryID, Message: text, Alert: alert}
	if _, err := b.api.MessagesSetBotCallbackAnswer(ctx, req); err != nil {
		b.log.Debug("answer callback failed", zap.Error(err))
	}
}

// ---- command parsing ----

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
	cmd = strings.ReplaceAll(cmd, "ё", "е")
	switch cmd {
	case "бан", "забанить", "banish":
		return "ban"
	case "разбан", "unban":
		return "unban"
	case "кик", "kick":
		return "kick"
	case "мут", "замут", "mute":
		return "mute"
	case "размут", "unmute":
		return "unmute"
	case "варн", "пред", "предупреждение", "warn":
		return "warn"
	case "снятьварн", "unwarn":
		return "unwarn"
	case "варны", "warns", "преды":
		return "warns"
	case "сбросварнов", "resetwarns":
		return "resetwarns"
	case "банлист", "banlist":
		return "banlist"
	case "муты", "mutelist":
		return "mutelist"
	case "админ", "addadmin", "назначить":
		return "addadmin"
	case "разжаловать", "unadmin", "снятьадмина":
		return "deladmin"
	case "админы", "admins", "стафф", "staff":
		return "admins"
	case "модер", "модератор", "addmoder", "addmoderator":
		return "addmoder"
	case "размодер", "размодератор", "снятьмодера", "unmoder", "delmoder", "delmoderator":
		return "delmoder"
	case "модеры", "модераторы", "moderators":
		return "moderators"
	case "моиправа", "myrights":
		return "myrights"
	case "настройки", "settings":
		return "settings"
	case "статистика", "stats", "стат":
		return "stats"
	case "правила", "rules":
		return "rules"
	case "установитьправила", "setrules":
		return "setrules"
	case "помощь", "help", "команды":
		return "help"
	case "старт", "start":
		return "start"
	case "ид", "id", "кто":
		return "id"
	case "айди", "айді", "userid":
		return "userid"
	case "инфо", "info", "профиль":
		return "info"
	case "пинг", "ping":
		return "ping"
	}
	return cmd
}

// ---- targets and roles ----

var (
	errNoTarget     = errors.New("no target")
	errBadTarget    = errors.New("bad target")
	errNoPermission = errors.New("no permission")
)

func (b *Bot) resolveTarget(ctx context.Context, ev *event, args []string) (*tg.User, []string, error) {
	if u := b.repliedUser(ctx, ev); u != nil {
		return u, args, nil
	}
	if len(args) == 0 {
		return nil, args, errNoTarget
	}
	token := args[0]
	u := b.resolveUserToken(ctx, ev.chatID, token)
	if u == nil || u.ID == 0 {
		return nil, args, errBadTarget
	}
	return u, args[1:], nil
}

// parseUserIDToken accepts a DeltaGram numeric ID or id<number> and rejects
// everything else. Moderation and staff commands must never guess a target from
// a Telegram username, a DeltaGram username or a display name: a wrong match
// would punish or promote the wrong person.
func parseUserIDToken(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	if token == "" || strings.HasPrefix(token, "@") {
		return 0, false
	}
	if len(token) > 2 && strings.EqualFold(token[:2], "id") {
		token = token[2:]
	}
	if !isDigits(token) {
		return 0, false
	}
	id, err := strconv.ParseInt(token, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// resolveUserToken resolves only a DeltaGram ID. Reply targets are handled
// separately in resolveTarget and stay supported.
func (b *Bot) resolveUserToken(ctx context.Context, chatID int64, token string) *tg.User {
	id, ok := parseUserIDToken(token)
	if !ok {
		return nil
	}
	return b.userInChat(ctx, chatID, id)
}

// findUser resolves a username or display name. It first tries the global
// username resolver, then falls back to the chat participant list, which also
// covers users who changed their username or have none at all.
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
	if u := b.cachedUserByName(query); u != nil {
		return u
	}
	if u, err := b.resolveUsername(ctx, query); err == nil && u != nil {
		return u
	}
	return b.searchParticipant(ctx, chatID, query)
}

func (b *Bot) cachedUserByName(query string) *tg.User {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	b.cache.mu.RLock()
	defer b.cache.mu.RUnlock()
	for _, u := range b.cache.users {
		if userMatches(u, query) {
			return u
		}
	}
	return nil
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
	// The ID itself is enough for moderation; the access hash is filled in by
	// the participant lookup when the server exposes it.
	return &tg.User{ID: userID}
}

func (b *Bot) participantByID(ctx context.Context, chatID, userID int64) *tg.User {
	ch := b.channel(ctx, chatID)
	if ch == nil {
		return nil
	}
	res, err := b.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Filter:  &tg.ChannelParticipantsRecent{},
		Offset:  0,
		Limit:   200,
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
	if ch == nil {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	filters := []tg.ChannelParticipantsFilterClass{
		&tg.ChannelParticipantsSearch{Q: query},
		&tg.ChannelParticipantsRecent{},
	}
	for _, filter := range filters {
		res, err := b.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
			Filter:  filter,
			Offset:  0,
			Limit:   200,
		})
		if err != nil {
			b.log.Debug("participant search failed", zap.Int64("chat", chatID), zap.Error(err))
			continue
		}
		data, ok := res.(*tg.ChannelsChannelParticipants)
		if !ok {
			continue
		}
		var users = make([]*tg.User, 0, len(data.Users))
		for _, uc := range data.Users {
			if u, ok := uc.(*tg.User); ok {
				b.cache.putUser(u)
				users = append(users, u)
			}
		}
		if u := matchUser(users, query); u != nil {
			return u
		}
	}
	return nil
}

// matchUser is the pure matcher behind name lookup. Exact username and full
// name wins over a partial first/last name match.
func matchUser(users []*tg.User, query string) *tg.User {
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, "@")))
	if q == "" {
		return nil
	}
	for _, u := range users {
		if u == nil {
			continue
		}
		if strings.EqualFold(u.Username, q) {
			return u
		}
	}
	for _, u := range users {
		if u == nil {
			continue
		}
		if strings.EqualFold(userFullName(u), q) {
			return u
		}
	}
	for _, u := range users {
		if u == nil {
			continue
		}
		if userMatches(u, q) {
			return u
		}
	}
	return nil
}

func userFullName(u *tg.User) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
}

func userMatches(u *tg.User, query string) bool {
	if u == nil {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, "@")))
	if q == "" {
		return false
	}
	if strings.EqualFold(u.Username, q) {
		return true
	}
	name := strings.ToLower(userFullName(u))
	if name == q {
		return true
	}
	first := strings.ToLower(strings.TrimSpace(u.FirstName))
	last := strings.ToLower(strings.TrimSpace(u.LastName))
	if first == q || last == q {
		return true
	}
	return strings.Contains(name, q) || strings.Contains(first, q)
}

// repliedUser resolves the author of the message the command replies to. It
// uses the in-memory message cache first, then channels.getMessages (whose
// users array carries the access hash), messages.getMessages for basic groups,
// and finally the reply header.
func (b *Bot) repliedUser(ctx context.Context, ev *event) *tg.User {
	if ev == nil || ev.msg == nil {
		return nil
	}
	hdr, ok := ev.msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || hdr.ReplyToMsgID <= 0 {
		return nil
	}
	if m := b.cache.getMessage(ev.chatID, hdr.ReplyToMsgID); m != nil {
		if u := b.userForMessage(ctx, ev.chatID, m); u != nil {
			return u
		}
	}
	if uid := peerUserID(hdr.ReplyFrom.FromID); uid != 0 {
		return b.userInChat(ctx, ev.chatID, uid)
	}
	// Try both message-lookup APIs regardless of chat kind. Supergroups answer
	// channels.getMessages, basic groups and private chats answer
	// messages.getMessages, and a migrated or unusual group may answer either,
	// so attempt both and return the first that resolves an author.
	if res, err := b.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: b.inputChannel(ev.chatID),
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: hdr.ReplyToMsgID}},
	}); err != nil {
		b.log.Warn("resolve replied message failed",
			zap.Int64("chat", ev.chatID), zap.Int("msg_id", hdr.ReplyToMsgID), zap.Error(err))
	} else if mod, ok := res.AsModified(); ok {
		for _, uc := range mod.GetUsers() {
			if u, ok := uc.(*tg.User); ok {
				b.cache.putUser(u)
			}
		}
		for _, mc := range mod.GetMessages() {
			if m, ok := mc.(*tg.Message); ok {
				b.rememberMessage(m)
				if u := b.userForMessage(ctx, ev.chatID, m); u != nil {
					return u
				}
			}
		}
	}
	// messages.getMessages recovers the author for basic groups and private
	// chats (and can also answer for a supergroup the bot has in its own box).
	if res, err := b.api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: hdr.ReplyToMsgID}}); err != nil {
		b.log.Warn("resolve replied message via box failed",
			zap.Int64("chat", ev.chatID), zap.Int("msg_id", hdr.ReplyToMsgID), zap.Error(err))
	} else if mod, ok := res.AsModified(); ok {
		for _, uc := range mod.GetUsers() {
			if u, ok := uc.(*tg.User); ok {
				b.cache.putUser(u)
			}
		}
		for _, mc := range mod.GetMessages() {
			if m, ok := mc.(*tg.Message); ok {
				b.rememberMessage(m)
				if u := b.userForMessage(ctx, ev.chatID, m); u != nil {
					return u
				}
			}
		}
	}
	return nil
}

func (b *Bot) userForMessage(ctx context.Context, chatID int64, m *tg.Message) *tg.User {
	if m == nil {
		return nil
	}
	uid := peerUserID(m.FromID)
	if uid == 0 {
		return nil
	}
	if u := b.cache.getUser(uid); u != nil {
		return u
	}
	if u := b.participantByID(ctx, chatID, uid); u != nil {
		return u
	}
	if u, err := b.user(ctx, uid); err == nil && u != nil {
		return u
	}
	return &tg.User{ID: uid}
}

func (b *Bot) repliedMessage(ctx context.Context, ev *event) *tg.Message {
	if ev == nil || ev.msg == nil || ev.kind != "channel" {
		return nil
	}
	hdr, ok := ev.msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || hdr.ReplyToMsgID <= 0 {
		return nil
	}
	if m := b.cache.getMessage(ev.chatID, hdr.ReplyToMsgID); m != nil {
		return m
	}
	res, err := b.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: b.inputChannel(ev.chatID),
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: hdr.ReplyToMsgID}},
	})
	if err != nil {
		b.log.Warn("resolve replied message failed",
			zap.Int64("chat", ev.chatID), zap.Int("msg_id", hdr.ReplyToMsgID), zap.Error(err))
		return nil
	}
	mod, ok := res.AsModified()
	if !ok {
		return nil
	}
	for _, uc := range mod.GetUsers() {
		if u, ok := uc.(*tg.User); ok {
			b.cache.putUser(u)
		}
	}
	for _, mc := range mod.GetMessages() {
		if m, ok := mc.(*tg.Message); ok {
			b.rememberMessage(m)
			return m
		}
	}
	return nil
}

type role int

const (
	roleNone role = iota
	roleMember
	roleAdmin
	roleCreator
	roleBanned
)

func (b *Bot) roleOf(ctx context.Context, chatID, userID int64) role {
	if chatID == 0 || userID == 0 {
		return roleNone
	}
	ch := b.channel(ctx, chatID)
	if ch == nil {
		return roleNone
	}
	res, err := b.api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
		Channel:     &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Participant: b.inputUser(ctx, userID),
	})
	if err == nil {
		switch p := res.Participant.(type) {
		case *tg.ChannelParticipantCreator:
			return roleCreator
		case *tg.ChannelParticipantAdmin:
			return roleAdmin
		case *tg.ChannelParticipantBanned:
			if p.BannedRights.ViewMessages {
				return roleBanned
			}
			return roleMember
		case *tg.ChannelParticipant:
			return roleMember
		}
	}
	// Fallback for servers that do not expose getParticipant to bots: scan the
	// admin list. This still lets the owner run the bot.
	admins, err := b.adminIDs(ctx, chatID)
	if err == nil {
		for _, id := range admins.admins {
			if id == userID {
				return roleAdmin
			}
		}
		if admins.creator == userID {
			return roleCreator
		}
	}
	return roleNone
}

type adminList struct {
	creator int64
	admins  []int64
}

func (b *Bot) adminIDs(ctx context.Context, chatID int64) (adminList, error) {
	var out adminList
	ch := b.channel(ctx, chatID)
	if ch == nil {
		return out, errBadTarget
	}
	res, err := b.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Filter:  &tg.ChannelParticipantsAdmins{},
		Offset:  0,
		Limit:   200,
	})
	if err != nil {
		return out, err
	}
	data, ok := res.(*tg.ChannelsChannelParticipants)
	if !ok {
		return out, nil
	}
	for _, pc := range data.Participants {
		switch p := pc.(type) {
		case *tg.ChannelParticipantCreator:
			out.creator = p.UserID
		case *tg.ChannelParticipantAdmin:
			out.admins = append(out.admins, p.UserID)
		}
	}
	return out, nil
}

func (b *Bot) actorRank(ctx context.Context, chatID, userID int64) int {
	if b.store.IsAdmin(chatID, userID) {
		return 2
	}
	if b.store.IsModerator(chatID, userID) {
		return 1
	}
	switch b.roleOf(ctx, chatID, userID) {
	case roleCreator:
		return 3
	case roleAdmin:
		return 2
	default:
		return 0
	}
}

func (b *Bot) isOwner(ctx context.Context, chatID, userID int64) bool {
	return b.roleOf(ctx, chatID, userID) == roleCreator
}

func (b *Bot) targetRank(ctx context.Context, chatID int64, target *tg.User) int {
	if target == nil {
		return 0
	}
	if b.store.IsAdmin(chatID, target.ID) {
		return 2
	}
	if b.store.IsModerator(chatID, target.ID) {
		return 1
	}
	switch b.roleOf(ctx, chatID, target.ID) {
	case roleCreator:
		return 3
	case roleAdmin:
		return 2
	default:
		return 0
	}
}

// ---- DeltaGram API helpers ----

func (b *Bot) channel(ctx context.Context, id int64) *tg.Channel {
	if id == 0 {
		return nil
	}
	if ch := b.cache.getChannel(id); ch != nil {
		return ch
	}
	res, err := b.api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{ChannelID: id},
	})
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
		return nil, errBadTarget
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
	return nil, errBadTarget
}

func (b *Bot) resolveUsername(ctx context.Context, username string) (*tg.User, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, errBadTarget
	}
	res, err := b.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, err
	}
	for _, uc := range res.Users {
		if u, ok := uc.(*tg.User); ok {
			b.cache.putUser(u)
			if strings.EqualFold(u.Username, username) {
				return u, nil
			}
		}
	}
	return nil, errBadTarget
}

func (b *Bot) channelHash(ctx context.Context, id int64) int64 {
	if ch := b.channel(ctx, id); ch != nil {
		return ch.AccessHash
	}
	return 0
}

func (b *Bot) inputChannel(id int64) tg.InputChannelClass {
	return &tg.InputChannel{ChannelID: id, AccessHash: b.channelHash(context.Background(), id)}
}

func (b *Bot) peerFor(chatID int64) tg.InputPeerClass {
	return &tg.InputPeerChannel{ChannelID: chatID, AccessHash: b.channelHash(context.Background(), chatID)}
}

func (b *Bot) inputUser(ctx context.Context, id int64) tg.InputPeerClass {
	var hash int64
	if u, err := b.user(ctx, id); err == nil && u != nil {
		hash = u.AccessHash
	}
	return &tg.InputPeerUser{UserID: id, AccessHash: hash}
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

func (b *Bot) sendEvent(ctx context.Context, ev *event, text string, markup tg.ReplyMarkupClass) {
	if ev == nil {
		return
	}
	var peer tg.InputPeerClass
	if ev.kind == "user" {
		peer = b.inputUser(ctx, ev.fromID)
	} else if ev.kind == "channel" {
		peer = b.peerFor(ev.chatID)
	}
	if peer == nil {
		return
	}
	replyTo := 0
	if ev.kind == "channel" {
		replyTo = ev.msg.ID
	}
	if _, err := b.send(ctx, peer, text, replyTo, markup); err != nil {
		b.log.Warn("send failed", zap.Int64("chat", ev.chatID), zap.Error(err))
	}
}

func (b *Bot) sendPrivate(ctx context.Context, userID int64, text string) {
	if userID == 0 {
		return
	}
	if _, err := b.send(ctx, b.inputUser(ctx, userID), text, 0, nil); err != nil {
		b.log.Warn("private send failed", zap.Int64("user", userID), zap.Error(err))
	}
}

func (b *Bot) editSettingsPanel(ctx context.Context, chatID int64, msgID int, settings Settings) {
	peer := b.peerFor(chatID)
	if peer == nil {
		return
	}
	_, err := b.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     settingsText(settings),
		ReplyMarkup: settingsMarkup(settings),
	})
	if err != nil {
		b.log.Debug("edit settings failed", zap.Error(err))
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

func displayName(u *tg.User) string {
	if u == nil {
		return "пользователь"
	}
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if name == "" && u.Username != "" {
		name = "@" + u.Username
	}
	if name == "" {
		name = fmt.Sprintf("id%d", u.ID)
	}
	return name
}

func userTag(u *tg.User) string {
	if u == nil {
		return "пользователь"
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return fmt.Sprintf("%s (id%d)", displayName(u), u.ID)
}

func truncateText(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + "…"
}
