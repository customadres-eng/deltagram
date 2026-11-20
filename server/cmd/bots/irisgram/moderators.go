package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"
)

type permissionDef struct {
	Key   string
	Label string
	Short string
}

var permissionDefs = []permissionDef{
	{PermBan, "Бан", "бан"},
	{PermUnban, "Разбан", "разбан"},
	{PermKick, "Кик", "кик"},
	{PermMute, "Мут", "мут"},
	{PermUnmute, "Снять мут", "размут"},
	{PermWarn, "Варн", "варн"},
	{PermUnwarn, "Снять варн", "снять варн"},
	{PermResetWarns, "Сброс варнов", "сброс варнов"},
	{PermLists, "Списки и статистика", "списки"},
}

func permissionLabel(key string) string {
	for _, def := range permissionDefs {
		if def.Key == key {
			return def.Label
		}
	}
	return key
}

func permissionsSummary(p ModeratorPermissions) string {
	labels := make([]string, 0, len(permissionDefs))
	for _, def := range permissionDefs {
		if p.Allows(def.Key) {
			labels = append(labels, def.Short)
		}
	}
	if len(labels) == 0 {
		return "нет прав"
	}
	return strings.Join(labels, ", ")
}

func (b *Bot) cmdAddModerator(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if target.ID == ev.fromID {
		b.sendEvent(ctx, ev, "Вы владелец чата и уже имеете все права.", nil)
		return
	}
	if b.store.IsAdmin(ev.chatID, target.ID) {
		b.sendEvent(ctx, ev, userTag(target)+" уже является админом бота (у него все права).", nil)
		return
	}
	created, err := b.store.AddModerator(ev.chatID, target.ID, displayName(target), ev.fromID)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось сохранить модератора.", nil)
		return
	}
	if !created {
		b.sendEvent(ctx, ev, userTag(target)+" уже является модератором. Откройте /moderators, чтобы изменить его права.", nil)
		return
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("✅ %s назначен модератором с полными правами.\n\nПрава можно изменить: /moderators или «Настройки» → «Модераторы».", userTag(target)), nil)
}

func (b *Bot) cmdDelModerator(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	removed, err := b.store.RemoveModerator(ev.chatID, target.ID)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось обновить модераторов.", nil)
		return
	}
	if !removed {
		b.sendEvent(ctx, ev, userTag(target)+" не является модератором.", nil)
		return
	}
	b.sendEvent(ctx, ev, "✅ "+userTag(target)+" больше не модератор.", nil)
}

func (b *Bot) cmdModerators(ctx context.Context, ev *event) {
	if !b.requireModerator(ctx, ev) {
		return
	}
	if !b.isOwner(ctx, ev.chatID, ev.fromID) {
		b.sendEvent(ctx, ev, moderatorListText(b, ctx, ev.chatID), nil)
		return
	}
	if peer := b.peerFor(ev.chatID); peer != nil {
		_, err := b.send(ctx, peer, moderatorListText(b, ctx, ev.chatID), ev.msg.ID, moderatorListMarkup(b.store.Moderators(ev.chatID)))
		if err != nil {
			b.logWarn("send moderators panel", err)
		}
	}
}

func (b *Bot) cmdMyRights(ctx context.Context, ev *event) {
	if ev.kind != "channel" {
		b.sendEvent(ctx, ev, "Команда работает только в супергруппах DeltaGram.", nil)
		return
	}
	rank := b.actorRank(ctx, ev.chatID, ev.fromID)
	switch rank {
	case 3:
		b.sendEvent(ctx, ev, "👑 Вы владелец чата. Доступны все команды модерации.", nil)
	case 2:
		b.sendEvent(ctx, ev, "🛡 Вы администратор чата или админ бота. Доступны все команды модерации.", nil)
	case 1:
		m, ok := b.store.Moderator(ev.chatID, ev.fromID)
		if !ok {
			b.sendEvent(ctx, ev, "Вы не являетесь модератором.", nil)
			return
		}
		b.sendEvent(ctx, ev, "🧑‍⚖️ Вы модератор.\nПрава: "+permissionsSummary(m.Permissions), nil)
	default:
		b.sendEvent(ctx, ev, "У вас нет прав модератора. Обратитесь к владельцу чата.", nil)
	}
}

func moderatorListText(b *Bot, ctx context.Context, chatID int64) string {
	mods := b.store.Moderators(chatID)
	lines := []string{"🧑‍⚖️ Модераторы чата"}
	if len(mods) == 0 {
		lines = append(lines, "\nПока не назначены.")
		lines = append(lines, "\nНазначить: /moder — ответом на сообщение или по ID DeltaGram.")
		return strings.Join(lines, "\n")
	}
	for _, m := range mods {
		lines = append(lines, fmt.Sprintf("• %s — %s", moderatorTag(b, ctx, chatID, m), permissionsSummary(m.Permissions)))
	}
	lines = append(lines, "\nНажмите на модератора, чтобы изменить его права.")
	return strings.Join(lines, "\n")
}

func moderatorTag(b *Bot, ctx context.Context, chatID int64, m Moderator) string {
	u := b.cache.getUser(m.UserID)
	if u == nil {
		if got, err := b.user(ctx, m.UserID); err == nil {
			u = got
		}
	}
	if u != nil && u.ID != 0 {
		return userTag(u)
	}
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	return fmt.Sprintf("id%d", m.UserID)
}

func moderatorCardText(b *Bot, ctx context.Context, chatID int64, m Moderator) string {
	lines := []string{
		"🧑‍⚖️ " + moderatorTag(b, ctx, chatID, m),
		fmt.Sprintf("ID: %d", m.UserID),
		"",
		"Нажмите на право, чтобы включить или выключить его.",
	}
	return strings.Join(lines, "\n")
}

func moderatorListMarkup(mods []Moderator) tg.ReplyMarkupClass {
	rows := make([]tg.KeyboardInlineButtonRow, 0, len(mods)+3)
	for _, m := range mods {
		label := strings.TrimSpace(m.Name)
		if label == "" {
			label = fmt.Sprintf("id%d", m.UserID)
		}
		if len([]rune(label)) > 28 {
			label = string([]rune(label)[:28]) + "…"
		}
		rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
			inlineButton("👤 "+label, fmt.Sprintf("md:open:%d", m.UserID)),
		}})
	}
	rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
		inlineButton("➕ Назначить модератора", "md:add"),
	}})
	rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
		inlineButton("⬅️ Назад", "md:back"),
		inlineButton("Закрыть", "md:close"),
	}})
	return &tg.ReplyInlineMarkup{Rows: rows}
}

func moderatorCardMarkup(m Moderator) tg.ReplyMarkupClass {
	rows := make([]tg.KeyboardInlineButtonRow, 0, len(permissionDefs)+2)
	for _, def := range permissionDefs {
		mark := "❌"
		if m.Permissions.Allows(def.Key) {
			mark = "✅"
		}
		rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
			inlineButton(mark+" "+def.Label, fmt.Sprintf("md:perm:%d:%s", m.UserID, def.Key)),
		}})
	}
	rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
		inlineButton("🗑 Снять модератора", fmt.Sprintf("md:del:%d", m.UserID)),
	}})
	rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: []tg.KeyboardInlineButton{
		inlineButton("⬅️ К списку", "md:list"),
		inlineButton("Закрыть", "md:close"),
	}})
	return &tg.ReplyInlineMarkup{Rows: rows}
}

func (b *Bot) editPanel(ctx context.Context, chatID int64, msgID int, text string, markup tg.ReplyMarkupClass) {
	peer := b.peerFor(chatID)
	if peer == nil {
		return
	}
	if _, err := b.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     text,
		ReplyMarkup: markup,
	}); err != nil {
		b.log.Debug("edit panel failed", zap.Error(err))
	}
}

func (b *Bot) handleModeratorCallback(ctx context.Context, q *tg.UpdateBotCallbackQuery, data string) {
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
		b.answerCallback(ctx, q.QueryID, "Менять модераторов может только владелец группы.", true)
		return
	}

	parts := strings.Split(data, ":")
	action := ""
	if len(parts) >= 2 {
		action = parts[1]
	}
	switch action {
	case "back":
		settings := b.store.Settings(chatID)
		b.editSettingsPanel(ctx, chatID, q.MsgID, settings)
		b.answerCallback(ctx, q.QueryID, "", false)
		return
	case "close":
		b.editPanel(ctx, chatID, q.MsgID, "🧑‍⚖️ Панель модераторов закрыта.", nil)
		b.answerCallback(ctx, q.QueryID, "Панель закрыта.", false)
		return
	case "add":
		b.answerCallback(ctx, q.QueryID, "Назначьте командой /moder — ответом на сообщение или по ID DeltaGram.", true)
		return
	case "list":
		b.editPanel(ctx, chatID, q.MsgID, moderatorListText(b, ctx, chatID), moderatorListMarkup(b.store.Moderators(chatID)))
		b.answerCallback(ctx, q.QueryID, "", false)
		return
	case "open":
		if len(parts) < 3 {
			b.answerCallback(ctx, q.QueryID, "Некорректная кнопка.", true)
			return
		}
		uid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || uid <= 0 {
			b.answerCallback(ctx, q.QueryID, "Некорректный пользователь.", true)
			return
		}
		m, ok := b.store.Moderator(chatID, uid)
		if !ok {
			b.answerCallback(ctx, q.QueryID, "Этот пользователь уже не модератор.", true)
			return
		}
		b.editPanel(ctx, chatID, q.MsgID, moderatorCardText(b, ctx, chatID, m), moderatorCardMarkup(m))
		b.answerCallback(ctx, q.QueryID, "", false)
		return
	case "perm":
		if len(parts) < 4 {
			b.answerCallback(ctx, q.QueryID, "Некорректная кнопка.", true)
			return
		}
		uid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || uid <= 0 {
			b.answerCallback(ctx, q.QueryID, "Некорректный пользователь.", true)
			return
		}
		m, ok := b.store.Moderator(chatID, uid)
		if !ok {
			b.answerCallback(ctx, q.QueryID, "Этот пользователь уже не модератор.", true)
			return
		}
		key := parts[3]
		updated, found, err := b.store.SetModeratorPermission(chatID, uid, key, !m.Permissions.Allows(key))
		if err != nil || !found {
			b.answerCallback(ctx, q.QueryID, "Не удалось сохранить право.", true)
			return
		}
		b.editPanel(ctx, chatID, q.MsgID, moderatorCardText(b, ctx, chatID, updated), moderatorCardMarkup(updated))
		state := "выключено"
		if updated.Permissions.Allows(key) {
			state = "включено"
		}
		b.answerCallback(ctx, q.QueryID, permissionLabel(key)+": "+state, false)
		return
	case "del":
		if len(parts) < 3 {
			b.answerCallback(ctx, q.QueryID, "Некорректная кнопка.", true)
			return
		}
		uid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || uid <= 0 {
			b.answerCallback(ctx, q.QueryID, "Некорректный пользователь.", true)
			return
		}
		removed, err := b.store.RemoveModerator(chatID, uid)
		if err != nil {
			b.answerCallback(ctx, q.QueryID, "Не удалось снять модератора.", true)
			return
		}
		b.editPanel(ctx, chatID, q.MsgID, moderatorListText(b, ctx, chatID), moderatorListMarkup(b.store.Moderators(chatID)))
		if removed {
			b.answerCallback(ctx, q.QueryID, "Модератор снят.", false)
		} else {
			b.answerCallback(ctx, q.QueryID, "Пользователь уже не модератор.", true)
		}
		return
	}
	b.answerCallback(ctx, q.QueryID, "Кнопка устарела.", true)
}
