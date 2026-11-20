package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"
)

const maxRulesLength = 3000

var (
	mutePresets = []int64{300, 900, 1800, 3600, 6 * 3600, 12 * 3600, 86400, 7 * 86400, 30 * 86400, 0}
	warnPresets = []int64{3600, 6 * 3600, 86400, 3 * 86400, 7 * 86400, 14 * 86400, 30 * 86400, 0}
	banPresets  = []int64{0, 3600, 6 * 3600, 86400, 7 * 86400, 30 * 86400}
)

func (b *Bot) dispatchPrivate(ctx context.Context, ev *event) {
	switch ev.cmd {
	case "start", "help":
		b.sendEvent(ctx, ev, privateIntroText(b.selfUser()), privateIntroMarkup(b.selfUser()))
	case "id":
		b.sendEvent(ctx, ev, b.idText(ctx, ev), nil)
	case "ping":
		b.sendEvent(ctx, ev, "🏓 Понг! Бот работает.", nil)
	default:
		// Unknown commands stay silent; the intro is only for /start and /help.
		return
	}
}

func (b *Bot) dispatch(ctx context.Context, ev *event) {
	if ev == nil || ev.msg == nil {
		return
	}
	switch ev.cmd {
	case "start":
		b.sendEvent(ctx, ev, "👋 Я на связи. Напишите /help, чтобы увидеть команды модерации.", nil)
	case "help":
		// The full command reference is staff-only; a regular member asking for
		// it is told they lack the rank instead of receiving the moderator manual.
		if !b.requireModerator(ctx, ev) {
			return
		}
		b.sendEvent(ctx, ev, helpText(), nil)
	case "ping":
		b.sendEvent(ctx, ev, "🏓 Понг! Бот работает.", nil)
	case "rules":
		b.cmdRules(ctx, ev)
	case "setrules":
		b.cmdSetRules(ctx, ev)
	case "settings", "setwarnlimit", "setwarntime", "setmutetime", "setbantime", "setwarnpunish":
		b.cmdSettings(ctx, ev)
	case "stats":
		b.cmdStats(ctx, ev)
	case "id":
		b.sendEvent(ctx, ev, b.idText(ctx, ev), nil)
	case "userid":
		b.cmdUserID(ctx, ev)
	case "info":
		b.cmdInfo(ctx, ev)
	case "admins":
		b.cmdAdmins(ctx, ev)
	case "addadmin":
		b.cmdAddAdmin(ctx, ev)
	case "deladmin":
		b.cmdDelAdmin(ctx, ev)
	case "addmoder":
		b.cmdAddModerator(ctx, ev)
	case "delmoder":
		b.cmdDelModerator(ctx, ev)
	case "moderators":
		b.cmdModerators(ctx, ev)
	case "myrights":
		b.cmdMyRights(ctx, ev)
	case "ban":
		b.cmdBan(ctx, ev)
	case "unban":
		b.cmdUnban(ctx, ev)
	case "kick":
		b.cmdKick(ctx, ev)
	case "mute":
		b.cmdMute(ctx, ev)
	case "unmute":
		b.cmdUnmute(ctx, ev)
	case "warn":
		b.cmdWarn(ctx, ev)
	case "unwarn":
		b.cmdUnwarn(ctx, ev)
	case "warns":
		b.cmdWarns(ctx, ev)
	case "resetwarns":
		b.cmdResetWarns(ctx, ev)
	case "banlist":
		b.cmdBanList(ctx, ev)
	case "mutelist":
		b.cmdMuteList(ctx, ev)
	default:
		// Unknown commands stay silent in groups: a moderation bot replying to
		// every unrelated /command would flood the chat.
		return
	}
}

func (b *Bot) cmdRules(ctx context.Context, ev *event) {
	rules := strings.TrimSpace(b.store.Rules(ev.chatID))
	if rules == "" {
		rules = "Правила чата пока не заданы. Владелец может задать их командой /setrules <текст>."
	}
	b.sendEvent(ctx, ev, "📜 Правила чата\n\n"+rules, nil)
}

func (b *Bot) cmdSetRules(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	text := strings.TrimSpace(ev.rest)
	if text == "" {
		b.sendEvent(ctx, ev, "Напишите текст правил: /setrules <текст>", nil)
		return
	}
	if len([]rune(text)) > maxRulesLength {
		b.sendEvent(ctx, ev, fmt.Sprintf("Слишком длинный текст правил. Максимум — %d символов.", maxRulesLength), nil)
		return
	}
	if _, err := b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.Rules = text }); err != nil {
		b.sendEvent(ctx, ev, "Не удалось сохранить правила. Попробуйте позже.", nil)
		return
	}
	b.sendEvent(ctx, ev, "✅ Правила чата сохранены.", nil)
}

func (b *Bot) cmdSettings(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	// Direct setters are kept for convenience; the buttons are the main UI.
	switch ev.cmd {
	case "setwarnlimit":
		if len(ev.args) == 1 {
			n, err := strconv.Atoi(ev.args[0])
			if err == nil && n >= 1 && n <= 20 {
				_, _ = b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.WarnLimit = n })
			}
		}
	case "setwarntime":
		if d, ok := parseDurationArg(firstArg(ev.args)); ok {
			_, _ = b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.WarnExpirySeconds = d })
		}
	case "setmutetime":
		if d, ok := parseDurationArg(firstArg(ev.args)); ok {
			_, _ = b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.MuteSeconds = d })
		}
	case "setbantime":
		if d, ok := parseDurationArg(firstArg(ev.args)); ok {
			_, _ = b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.BanSeconds = d })
		}
	case "setwarnpunish":
		if p := strings.ToLower(firstArg(ev.args)); p == "mute" || p == "ban" || p == "kick" || p == "мут" || p == "бан" || p == "кик" {
			switch p {
			case "мут":
				p = "mute"
			case "бан":
				p = "ban"
			case "кик":
				p = "kick"
			}
			_, _ = b.store.UpdateSettings(ev.chatID, func(s *Settings) { s.WarnPunish = p })
		}
	}
	settings := b.store.Settings(ev.chatID)
	peer := b.peerFor(ev.chatID)
	if peer == nil {
		return
	}
	if _, err := b.send(ctx, peer, settingsText(settings), ev.msg.ID, settingsMarkup(settings)); err != nil {
		b.logWarn("send settings", err)
	}
}

func (b *Bot) cmdStats(ctx context.Context, ev *event) {
	// "/статистика чат" keeps the old chat-wide report; plain "/статистика"
	// replies to the invoker with their own full profile stats.
	if firstArg(ev.args) == "чат" || firstArg(ev.args) == "chat" {
		b.chatStats(ctx, ev)
		return
	}
	if ev.fromID == 0 {
		b.sendEvent(ctx, ev, "Не удалось определить пользователя.", nil)
		return
	}
	text := b.personalStatsText(ctx, ev, ev.fromID)
	// Reply to the command message so the requester sees their own numbers.
	b.sendEvent(ctx, ev, text, nil)
}

func (b *Bot) chatStats(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermLists) {
		return
	}
	b.sendEvent(ctx, ev, b.chatStatsText(ctx, ev.chatID), nil)
}

func (b *Bot) chatStatsText(ctx context.Context, chatID int64) string {
	st := b.store.Stats(chatID)
	settings := b.store.Settings(chatID)
	return fmt.Sprintf("📊 Статистика чата\n\n"+
		"👮 Команда модерации\n"+
		"• Админов бота: %d\n"+
		"• Модераторов: %d\n\n"+
		"⚠️ Нарушения\n"+
		"• Активных предупреждений: %d\n"+
		"• Забанено: %d\n"+
		"• В муте: %d\n\n"+
		"⚙️ Настройки наказаний\n"+
		"• Лимит варнов: %d\n"+
		"• Наказание при лимите: %s",
		st.Admins, st.Moderators, st.Warns, st.Bans, st.Mutes, settings.WarnLimit, punishLabel(settings.WarnPunish))
}

// personalStatsText builds the reply for /статистика. Data comes from the
// server database through the read-only botdata reader; when it is unavailable
// the bot still answers with what it knows locally (warns and role).
func (b *Bot) personalStatsText(ctx context.Context, ev *event, userID int64) string {
	lines := []string{fmt.Sprintf("📊 Ваша статистика — %s", userTag(ev.from)), ""}
	haveData := false
	if b.data != nil {
		stats, err := b.data.Stats(ctx, userID)
		if err == nil {
			haveData = true
			lines = append(lines,
				"👤 Аккаунт",
				fmt.Sprintf("• ID: %d", stats.UserID))
			if !stats.CreatedAt.IsZero() {
				lines = append(lines, "• В DeltaGram с: "+stats.CreatedAt.Format("02.01.2006"))
			}
			if stats.PremiumUntil.After(time.Now()) {
				lines = append(lines, "• Premium до: "+stats.PremiumUntil.Format("02.01.2006"))
			}
			lines = append(lines,
				"",
				"⭐ Звёзды",
				"• Баланс: "+formatCount(stats.Stars),
				"",
				"🎁 Подарки в профиле",
				"• Всего: "+formatCount(stats.Gifts.Total),
				"• Улучшено до уникальных: "+formatCount(stats.Gifts.Collectible),
				"• На чёрном фоне: "+formatCount(stats.Gifts.BlackBackdrop),
				"",
				"💬 Сообщения",
				"• Всего: "+formatCount(stats.Messages()),
				"• В личных чатах: "+formatCount(stats.PrivateMessages),
				"• В группах и каналах: "+formatCount(stats.ChannelMessages))
		} else {
			b.logWarn("load personal stats", err)
		}
	}
	if !haveData {
		lines = append(lines, "📈 Данные аккаунта временно недоступны. Попробуйте позже.")
	}
	if ev.kind != "user" {
		settings := b.store.Settings(ev.chatID)
		warns := len(b.store.Warns(ev.chatID, userID))
		lines = append(lines,
			"",
			"🛡 В этом чате",
			"• Роль: "+b.roleLabel(ctx, ev.chatID, userID),
			fmt.Sprintf("• Предупреждения: %d из %d", warns, settings.WarnLimit))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (b *Bot) roleLabel(ctx context.Context, chatID, userID int64) string {
	if b.store.IsAdmin(chatID, userID) {
		return "админ бота"
	}
	if b.store.IsModerator(chatID, userID) {
		return "модератор"
	}
	switch b.roleOf(ctx, chatID, userID) {
	case roleCreator:
		return "владелец чата"
	case roleAdmin:
		return "администратор"
	default:
		return "участник"
	}
}

func formatCount(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var out strings.Builder
	out.WriteString(sign)
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(' ')
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (b *Bot) idText(ctx context.Context, ev *event) string {
	// Когда команда отправлена в ответ на чужое сообщение, нужен ID автора
	// того сообщения, а не автора команды. Только если ответа нет — показываем
	// собственный ID отправителя.
	target := b.repliedUser(ctx, ev)
	if target == nil {
		target = ev.from
	}
	lines := []string{"🪪 Информация"}
	if target != nil {
		lines = append(lines, fmt.Sprintf("Пользователь: %s\nID: %d", displayName(target), target.ID))
	} else if ev.from != nil {
		lines = append(lines, fmt.Sprintf("Пользователь: %s\nID: %d", displayName(ev.from), ev.fromID))
	} else {
		lines = append(lines, fmt.Sprintf("ID пользователя: %d", ev.fromID))
	}
	if ev.kind == "channel" {
		name := ""
		if ch := b.channel(ctx, ev.chatID); ch != nil {
			name = ch.Title
		}
		if name != "" {
			lines = append(lines, fmt.Sprintf("Чат: %s\nID чата: %d", name, ev.chatID))
		} else {
			lines = append(lines, fmt.Sprintf("ID чата: %d", ev.chatID))
		}
	}
	return strings.Join(lines, "\n")
}

// cmdUserID replies with the numeric DeltaGram ID of the user a moderator
// replied to, formatted so it can be copied straight into another command.
// Every rank above an ordinary member may use it.
func (b *Bot) cmdUserID(ctx context.Context, ev *event) {
	if !b.requireModerator(ctx, ev) {
		return
	}
	replying := false
	if ev.msg != nil {
		if hdr, ok := ev.msg.ReplyTo.(*tg.MessageReplyHeader); ok && hdr.ReplyToMsgID > 0 {
			replying = true
		}
	}
	target := b.repliedUser(ctx, ev)
	if target == nil && len(ev.args) > 0 {
		target = b.resolveUserToken(ctx, ev.chatID, ev.args[0])
	}
	if target == nil && replying {
		// Never silently answer with the caller's own ID: that made /айди look
		// like it returned "my ID" instead of the replied user's.
		b.sendEvent(ctx, ev, "Не удалось определить автора этого сообщения. Попробуйте ответить ещё раз или укажите ID: /айди 1780243200", nil)
		return
	}
	if target == nil {
		// Without a reply the useful fallback is the caller's own ID.
		target = ev.from
	}
	if target == nil || target.ID == 0 {
		b.sendEvent(ctx, ev, "Ответьте на сообщение пользователя, чтобы получить его ID DeltaGram.", nil)
		return
	}
	who := displayName(target)
	if target.Username != "" {
		who = "@" + target.Username
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("🪪 ID DeltaGram: %d\n%s\n\nСкопируйте это число и используйте в командах, например: /мут %d", target.ID, who, target.ID), nil)
}

func (b *Bot) cmdInfo(ctx context.Context, ev *event) {
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		target = ev.from
	}
	if target == nil {
		b.sendEvent(ctx, ev, "Не удалось определить пользователя.", nil)
		return
	}
	var lines []string
	lines = append(lines, "👤 Профиль")
	lines = append(lines, "Имя: "+displayName(target))
	if target.Username != "" {
		lines = append(lines, "Username: @"+target.Username)
	}
	lines = append(lines, fmt.Sprintf("ID: %d", target.ID))
	switch b.roleOf(ctx, ev.chatID, target.ID) {
	case roleCreator:
		lines = append(lines, "Роль: владелец чата")
	case roleAdmin:
		lines = append(lines, "Роль: администратор")
	case roleBanned:
		lines = append(lines, "Статус: забанен")
	default:
		lines = append(lines, "Роль: участник")
	}
	if b.store.IsAdmin(ev.chatID, target.ID) {
		lines = append(lines, "Админ бота: да")
	}
	if m, ok := b.store.Moderator(ev.chatID, target.ID); ok {
		lines = append(lines, "Модератор бота: да ("+permissionsSummary(m.Permissions)+")")
	}
	warns := b.store.Warns(ev.chatID, target.ID)
	lines = append(lines, fmt.Sprintf("Предупреждений: %d", len(warns)))
	if rec, ok := b.store.Ban(ev.chatID, target.ID); ok {
		lines = append(lines, "Бан: "+formatRecordUntil(rec.Until))
	}
	if rec, ok := b.store.Mute(ev.chatID, target.ID); ok {
		lines = append(lines, "Мут: "+formatRecordUntil(rec.Until))
	}
	b.sendEvent(ctx, ev, strings.Join(lines, "\n"), nil)
}

func (b *Bot) cmdAdmins(ctx context.Context, ev *event) {
	var lines []string
	lines = append(lines, "👮 Администрация чата")
	botAdmins := b.store.Admins(ev.chatID)
	if len(botAdmins) == 0 {
		lines = append(lines, "\nАдмины бота: не назначены.")
	} else {
		lines = append(lines, "\nАдмины бота:")
		for _, id := range botAdmins {
			u, _ := b.user(ctx, id)
			lines = append(lines, "• "+userTag(u))
		}
	}
	mods := b.store.Moderators(ev.chatID)
	if len(mods) == 0 {
		lines = append(lines, "\nМодераторы бота: не назначены.")
	} else {
		lines = append(lines, "\nМодераторы бота:")
		for _, m := range mods {
			lines = append(lines, fmt.Sprintf("• %s — %s", moderatorTag(b, ctx, ev.chatID, m), permissionsSummary(m.Permissions)))
		}
	}
	if list, err := b.adminIDs(ctx, ev.chatID); err == nil {
		lines = append(lines, "\nАдминистраторы DeltaGram:")
		if list.creator != 0 {
			u, _ := b.user(ctx, list.creator)
			lines = append(lines, "• "+userTag(u)+" — владелец")
		}
		count := 0
		for _, id := range list.admins {
			if b.store.IsAdmin(ev.chatID, id) {
				continue
			}
			u, _ := b.user(ctx, id)
			lines = append(lines, "• "+userTag(u))
			count++
			if count >= 25 {
				lines = append(lines, "…")
				break
			}
		}
		if list.creator == 0 && count == 0 {
			lines = append(lines, "• нет данных")
		}
	}
	b.sendEvent(ctx, ev, strings.Join(lines, "\n"), nil)
}

func (b *Bot) cmdAddAdmin(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if target.ID == ev.fromID {
		b.sendEvent(ctx, ev, "Вы уже владелец чата.", nil)
		return
	}
	added, err := b.store.AddAdmin(ev.chatID, target.ID)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось сохранить администратора.", nil)
		return
	}
	if !added {
		b.sendEvent(ctx, ev, userTag(target)+" уже является админом бота.", nil)
		return
	}
	b.sendEvent(ctx, ev, "✅ "+userTag(target)+" назначен админом бота.", nil)
}

func (b *Bot) cmdDelAdmin(ctx context.Context, ev *event) {
	if !b.requireOwner(ctx, ev) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	removed, err := b.store.RemoveAdmin(ev.chatID, target.ID)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось обновить администраторов.", nil)
		return
	}
	if !removed {
		b.sendEvent(ctx, ev, userTag(target)+" не является админом бота.", nil)
		return
	}
	b.sendEvent(ctx, ev, "✅ "+userTag(target)+" больше не админ бота.", nil)
}

func (b *Bot) cmdBan(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermBan) {
		return
	}
	target, rest, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if !b.canPunish(ctx, ev, target) {
		return
	}
	settings := b.store.Settings(ev.chatID)
	dur := settings.BanSeconds
	if d, left, ok := parseDurationLoose(rest); ok {
		dur, rest = d, left
	}
	reason := strings.Join(rest, " ")
	if err := b.ban(ctx, ev, target, dur, reason); err != nil {
		b.actionFailed(ctx, ev, "забанить", err)
		return
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("🚫 %s забанен%s. %s", userTag(target), reasonSuffix(reason), durationSuffix(dur)), nil)
}

func (b *Bot) cmdUnban(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermUnban) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if err := b.unban(ctx, ev, target); err != nil {
		b.actionFailed(ctx, ev, "разбанить", err)
		return
	}
	b.sendEvent(ctx, ev, "✅ "+userTag(target)+" разбанен.", nil)
}

func (b *Bot) cmdKick(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermKick) {
		return
	}
	target, rest, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if !b.canPunish(ctx, ev, target) {
		return
	}
	reason := strings.Join(rest, " ")
	if err := b.kick(ctx, ev, target); err != nil {
		b.actionFailed(ctx, ev, "кикнуть", err)
		return
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("👢 %s удалён из чата%s.", userTag(target), reasonSuffix(reason)), nil)
}

func (b *Bot) cmdMute(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermMute) {
		return
	}
	target, rest, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if !b.canPunish(ctx, ev, target) {
		return
	}
	settings := b.store.Settings(ev.chatID)
	dur := settings.MuteSeconds
	if d, left, ok := parseDurationLoose(rest); ok {
		dur, rest = d, left
	}
	reason := strings.Join(rest, " ")
	if err := b.mute(ctx, ev, target, dur, reason); err != nil {
		b.actionFailed(ctx, ev, "замутить", err)
		return
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("🔇 %s замучен%s. %s", userTag(target), reasonSuffix(reason), durationSuffix(dur)), nil)
}

func (b *Bot) cmdUnmute(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermUnmute) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if err := b.unmute(ctx, ev, target); err != nil {
		b.actionFailed(ctx, ev, "снять мут", err)
		return
	}
	b.sendEvent(ctx, ev, "✅ С "+userTag(target)+" снят мут.", nil)
}

func (b *Bot) cmdWarn(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermWarn) {
		return
	}
	target, rest, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if !b.canPunish(ctx, ev, target) {
		return
	}
	reason := strings.Join(rest, " ")
	settings := b.store.Settings(ev.chatID)
	now := time.Now()
	expires := int64(0)
	if settings.WarnExpirySeconds > 0 {
		expires = now.Add(time.Duration(settings.WarnExpirySeconds) * time.Second).Unix()
	}
	w := Warn{At: now.Unix(), ExpiresAt: expires, By: ev.fromID, ByName: displayName(ev.from), Reason: truncateText(reason, 300)}
	total, reached, err := b.store.AddWarn(ev.chatID, target.ID, w, settings.WarnLimit)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось сохранить предупреждение.", nil)
		return
	}
	if !reached {
		b.sendEvent(ctx, ev, fmt.Sprintf("⚠️ %s получил предупреждение (%d/%d)%s.",
			userTag(target), total, settings.WarnLimit, reasonSuffix(reason)), nil)
		return
	}
	var actionErr error
	switch settings.WarnPunish {
	case "ban":
		actionErr = b.ban(ctx, ev, target, settings.BanSeconds, "лимит предупреждений")
	case "kick":
		actionErr = b.kick(ctx, ev, target)
	default:
		actionErr = b.mute(ctx, ev, target, settings.WarnMuteSeconds, "лимит предупреждений")
	}
	if actionErr != nil {
		b.sendEvent(ctx, ev, fmt.Sprintf("⚠️ %s достиг лимита предупреждений (%d/%d), но наказание не удалось: %s",
			userTag(target), total, settings.WarnLimit, apiErrorText(actionErr)), nil)
		return
	}
	_ = b.store.ClearWarns(ev.chatID, target.ID)
	b.sendEvent(ctx, ev, fmt.Sprintf("⚠️ %s достиг лимита предупреждений (%d/%d). Применено наказание: %s.",
		userTag(target), total, settings.WarnLimit, punishLabel(settings.WarnPunish)), nil)
}

func (b *Bot) cmdUnwarn(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermUnwarn) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	left, removed, err := b.store.RemoveLastWarn(ev.chatID, target.ID)
	if err != nil {
		b.sendEvent(ctx, ev, "Не удалось обновить предупреждения.", nil)
		return
	}
	if !removed {
		b.sendEvent(ctx, ev, "У "+userTag(target)+" нет предупреждений.", nil)
		return
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("✅ Снято одно предупреждение с %s. Осталось: %d.", userTag(target), left), nil)
}

func (b *Bot) cmdWarns(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermLists) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		target = ev.from
	}
	if target == nil {
		return
	}
	warns := b.store.Warns(ev.chatID, target.ID)
	if len(warns) == 0 {
		b.sendEvent(ctx, ev, "У "+userTag(target)+" нет предупреждений.", nil)
		return
	}
	lines := []string{fmt.Sprintf("⚠️ Предупреждения %s: %d", userTag(target), len(warns))}
	for i, w := range warns {
		line := fmt.Sprintf("%d. %s", i+1, time.Unix(w.At, 0).Format("02.01.2006 15:04"))
		if w.Reason != "" {
			line += " — " + truncateText(w.Reason, 100)
		}
		if w.ExpiresAt > 0 {
			line += " (до " + time.Unix(w.ExpiresAt, 0).Format("02.01.2006") + ")"
		}
		lines = append(lines, line)
	}
	b.sendEvent(ctx, ev, strings.Join(lines, "\n"), nil)
}

func (b *Bot) cmdResetWarns(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermResetWarns) {
		return
	}
	target, _, err := b.resolveTarget(ctx, ev, ev.args)
	if err != nil {
		b.sendEvent(ctx, ev, "Укажите пользователя ответом на сообщение или его ID DeltaGram.", nil)
		return
	}
	if !b.canPunish(ctx, ev, target) {
		return
	}
	if err := b.store.ClearWarns(ev.chatID, target.ID); err != nil {
		b.sendEvent(ctx, ev, "Не удалось сбросить предупреждения.", nil)
		return
	}
	b.sendEvent(ctx, ev, "✅ Все предупреждения "+userTag(target)+" сброшены.", nil)
}

func (b *Bot) cmdBanList(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermLists) {
		return
	}
	bans := b.store.Bans(ev.chatID)
	if len(bans) == 0 {
		b.sendEvent(ctx, ev, "🚫 Список банов пуст.", nil)
		return
	}
	ids := make([]int64, 0, len(bans))
	for id := range bans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	users := b.usersByIDs(ctx, ids)
	lines := []string{fmt.Sprintf("🚫 Забаненные (%d):", len(ids))}
	for i, id := range ids {
		if i >= 40 {
			lines = append(lines, "…")
			break
		}
		rec := bans[id]
		line := "• " + userTag(users[id])
		if rec.Reason != "" {
			line += " — " + truncateText(rec.Reason, 80)
		}
		line += " (" + formatRecordUntil(rec.Until) + ")"
		lines = append(lines, line)
	}
	b.sendEvent(ctx, ev, strings.Join(lines, "\n"), nil)
}

func (b *Bot) cmdMuteList(ctx context.Context, ev *event) {
	if !b.requirePermission(ctx, ev, PermLists) {
		return
	}
	mutes := b.store.Mutes(ev.chatID)
	if len(mutes) == 0 {
		b.sendEvent(ctx, ev, "🔇 Список мутов пуст.", nil)
		return
	}
	ids := make([]int64, 0, len(mutes))
	for id := range mutes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	users := b.usersByIDs(ctx, ids)
	lines := []string{fmt.Sprintf("🔇 В муте (%d):", len(ids))}
	for i, id := range ids {
		if i >= 40 {
			lines = append(lines, "…")
			break
		}
		rec := mutes[id]
		line := "• " + userTag(users[id])
		if rec.Reason != "" {
			line += " — " + truncateText(rec.Reason, 80)
		}
		line += " (" + formatRecordUntil(rec.Until) + ")"
		lines = append(lines, line)
	}
	b.sendEvent(ctx, ev, strings.Join(lines, "\n"), nil)
}

// ---- permissions ----

func (b *Bot) requireModerator(ctx context.Context, ev *event) bool {
	if ev.kind != "channel" {
		b.sendEvent(ctx, ev, "Модерация доступна только в супергруппах DeltaGram.", nil)
		return false
	}
	if b.actorRank(ctx, ev.chatID, ev.fromID) < 1 {
		b.sendEvent(ctx, ev, "⛔ Недостаточно прав. Модерировать могут владелец, администраторы чата, админы и модераторы бота.", nil)
		return false
	}
	return true
}

// requirePermission allows owner, Telegram admins and bot admins through, and
// checks the individual right for bot-appointed moderators.
func (b *Bot) requirePermission(ctx context.Context, ev *event, permission string) bool {
	if ev.kind != "channel" {
		b.sendEvent(ctx, ev, "Модерация доступна только в супергруппах DeltaGram.", nil)
		return false
	}
	rank := b.actorRank(ctx, ev.chatID, ev.fromID)
	if rank >= 2 {
		return true
	}
	if rank == 1 && b.store.ModeratorAllows(ev.chatID, ev.fromID, permission) {
		return true
	}
	if rank == 1 {
		b.sendEvent(ctx, ev, fmt.Sprintf("⛔ У вас нет права «%s». Владелец может выдать его в настройках модераторов.", permissionLabel(permission)), nil)
		return false
	}
	b.sendEvent(ctx, ev, "⛔ Недостаточно прав. Модерировать могут владелец, администраторы чата, админы и модераторы бота.", nil)
	return false
}

func (b *Bot) requireOwner(ctx context.Context, ev *event) bool {
	if ev.kind != "channel" {
		b.sendEvent(ctx, ev, "Эта команда доступна только в супергруппах DeltaGram.", nil)
		return false
	}
	if !b.isOwner(ctx, ev.chatID, ev.fromID) {
		b.sendEvent(ctx, ev, "⛔ Эта команда доступна только владельцу группы.", nil)
		return false
	}
	return true
}

func (b *Bot) canPunish(ctx context.Context, ev *event, target *tg.User) bool {
	if target == nil || target.ID == 0 {
		b.sendEvent(ctx, ev, "Не удалось определить пользователя.", nil)
		return false
	}
	if self := b.selfUser(); self != nil && target.ID == self.ID {
		b.sendEvent(ctx, ev, "Себя я наказывать не буду 🙂", nil)
		return false
	}
	actor := b.actorRank(ctx, ev.chatID, ev.fromID)
	targetRank := b.targetRank(ctx, ev.chatID, target)
	if targetRank >= actor {
		b.sendEvent(ctx, ev, "⛔ Нельзя наказать пользователя с равными или более высокими правами.", nil)
		return false
	}
	return true
}

// ---- moderation actions ----

func (b *Bot) ban(ctx context.Context, ev *event, target *tg.User, seconds int64, reason string) error {
	until := untilUnix(seconds)
	rights := tg.ChatBannedRights{ViewMessages: true, UntilDate: until}
	if err := b.editBanned(ctx, ev.chatID, target, rights); err != nil {
		return err
	}
	_ = b.store.PutBan(ev.chatID, target.ID, Record{
		At: time.Now().Unix(), Until: int64(until), By: ev.fromID,
		ByName: displayName(ev.from), Reason: truncateText(reason, 300),
	})
	return nil
}

func (b *Bot) unban(ctx context.Context, ev *event, target *tg.User) error {
	if err := b.editBanned(ctx, ev.chatID, target, tg.ChatBannedRights{}); err != nil {
		return err
	}
	_ = b.store.DeleteBan(ev.chatID, target.ID)
	return nil
}

func (b *Bot) kick(ctx context.Context, ev *event, target *tg.User) error {
	if err := b.editBanned(ctx, ev.chatID, target, tg.ChatBannedRights{ViewMessages: true}); err != nil {
		return err
	}
	if err := b.editBanned(ctx, ev.chatID, target, tg.ChatBannedRights{}); err != nil {
		return err
	}
	_ = b.store.DeleteBan(ev.chatID, target.ID)
	return nil
}

func (b *Bot) mute(ctx context.Context, ev *event, target *tg.User, seconds int64, reason string) error {
	until := untilUnix(seconds)
	rights := tg.ChatBannedRights{
		SendMessages:    true,
		SendMedia:       true,
		SendStickers:    true,
		SendGifs:        true,
		SendGames:       true,
		SendInline:      true,
		EmbedLinks:      true,
		SendPolls:       true,
		SendPhotos:      true,
		SendVideos:      true,
		SendRoundvideos: true,
		SendAudios:      true,
		SendVoices:      true,
		SendDocs:        true,
		SendPlain:       true,
		SendReactions:   true,
		UntilDate:       until,
	}
	if err := b.editBanned(ctx, ev.chatID, target, rights); err != nil {
		return err
	}
	_ = b.store.PutMute(ev.chatID, target.ID, Record{
		At: time.Now().Unix(), Until: int64(until), By: ev.fromID,
		ByName: displayName(ev.from), Reason: truncateText(reason, 300),
	})
	return nil
}

func (b *Bot) unmute(ctx context.Context, ev *event, target *tg.User) error {
	if err := b.editBanned(ctx, ev.chatID, target, tg.ChatBannedRights{}); err != nil {
		return err
	}
	_ = b.store.DeleteMute(ev.chatID, target.ID)
	return nil
}

func (b *Bot) editBanned(ctx context.Context, chatID int64, target *tg.User, rights tg.ChatBannedRights) error {
	ch := b.channel(ctx, chatID)
	if ch == nil {
		return fmt.Errorf("chat not found")
	}
	_, err := b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel:      &tg.InputChannel{ChannelID: chatID, AccessHash: ch.AccessHash},
		Participant:  b.inputUser(ctx, target.ID),
		BannedRights: rights,
	})
	return err
}

func (b *Bot) actionFailed(ctx context.Context, ev *event, action string, err error) {
	hint := ""
	msg := apiErrorText(err)
	if strings.Contains(strings.ToLower(msg), "admin") || strings.Contains(strings.ToLower(msg), "right") {
		hint = "\n\nПроверьте, что у меня есть права администратора с разрешением «Блокировать участников»."
	}
	b.sendEvent(ctx, ev, fmt.Sprintf("Не удалось %s: %s%s", action, msg, hint), nil)
}

func (b *Bot) logWarn(msg string, err error) {
	if b.log != nil {
		b.log.Warn(msg, zap.Error(err))
	}
}

// ---- small helpers ----

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func untilUnix(seconds int64) int {
	if seconds <= 0 {
		return 0
	}
	if seconds > 365*86400 {
		seconds = 365 * 86400
	}
	return int(time.Now().Add(time.Duration(seconds) * time.Second).Unix())
}

func formatRecordUntil(until int64) string {
	if until <= 0 {
		return "навсегда"
	}
	return "до " + time.Unix(until, 0).Format("02.01.2006 15:04")
}

func durationSuffix(seconds int64) string {
	if seconds <= 0 {
		return "Навсегда."
	}
	return "Срок: " + formatDuration(seconds) + "."
}

func reasonSuffix(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	return ", причина: " + truncateText(reason, 200)
}

func punishLabel(v string) string {
	switch normalizeWarnPunish(v) {
	case "ban":
		return "бан"
	case "kick":
		return "кик"
	default:
		return "мут"
	}
}

func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return "навсегда"
	}
	switch {
	case seconds%86400 == 0:
		return fmt.Sprintf("%d дн.", seconds/86400)
	case seconds%3600 == 0:
		return fmt.Sprintf("%d ч.", seconds/3600)
	case seconds%60 == 0:
		return fmt.Sprintf("%d мин.", seconds/60)
	default:
		return fmt.Sprintf("%d сек.", seconds)
	}
}

func parseDurationArg(s string) (int64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, false
	}
	switch s {
	case "0", "навсегда", "вечно", "бессрочно", "перманентно", "forever":
		return 0, true
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, false
	}
	switch s[i:] {
	case "", "м", "мин", "минута", "минуты", "минут", "m", "min", "minute", "minutes":
		n *= 60
	case "с", "сек", "секунда", "секунды", "секунд", "s", "sec", "second", "seconds":
	case "ч", "час", "часа", "часов", "h", "hour", "hours":
		n *= 3600
	case "д", "дн", "день", "дня", "дней", "d", "day", "days":
		n *= 86400
	case "н", "нед", "неделя", "недели", "недель", "w", "week", "weeks":
		n *= 7 * 86400
	case "мес", "месяц", "месяца", "месяцев", "mo", "month", "months":
		n *= 30 * 86400
	default:
		return 0, false
	}
	return n, true
}

func parseDurationLoose(args []string) (int64, []string, bool) {
	if len(args) == 0 {
		return 0, args, false
	}
	// "10 минут", "2 часа", "1 день": the unit is frequently a separate word.
	// Prefer the combined form over the bare-number default (minutes).
	if isDigits(args[0]) && len(args) >= 2 {
		if d, ok := parseDurationArg(args[0] + args[1]); ok {
			return d, args[2:], true
		}
		if d, ok := parseDurationArg(args[0] + " " + args[1]); ok {
			return d, args[2:], true
		}
	}
	if d, ok := parseDurationArg(args[0]); ok {
		return d, args[1:], true
	}
	return 0, args, false
}

func nextDuration(cur int64, presets []int64) int64 {
	for _, v := range presets {
		if v > cur {
			return v
		}
	}
	if len(presets) == 0 {
		return cur
	}
	return presets[0]
}

func apiErrorText(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	// Keep the API code, drop noisy transport wrapping.
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		tail := msg[i+2:]
		if strings.Contains(tail, "_") || strings.EqualFold(tail, "forbidden") {
			return tail
		}
	}
	return truncateText(msg, 240)
}

func (b *Bot) usersByIDs(ctx context.Context, ids []int64) map[int64]*tg.User {
	out := map[int64]*tg.User{}
	if len(ids) == 0 {
		return out
	}
	if len(ids) > 100 {
		ids = ids[:100]
	}
	inputs := make([]tg.InputUserClass, 0, len(ids))
	for _, id := range ids {
		inputs = append(inputs, &tg.InputUser{UserID: id})
	}
	list, err := b.api.UsersGetUsers(ctx, inputs)
	if err != nil {
		return out
	}
	for _, uc := range list {
		if u, ok := uc.(*tg.User); ok {
			b.cache.putUser(u)
			out[u.ID] = u
		}
	}
	return out
}

// ---- texts ----

func privateIntroText(self *tg.User) string {
	name := "Ирис"
	if self != nil && strings.TrimSpace(self.FirstName) != "" {
		name = strings.TrimSpace(self.FirstName)
	}
	return fmt.Sprintf("👋 Привет! Я %s — бот-модератор DeltaGram.\n\n"+
		"Нажмите кнопку ниже — я добавлюсь в группу сразу с полными правами администратора.\n\n"+
		"Если добавляете вручную, включите право «Блокировать участников».\n\n"+
		"Что я умею:\n"+
		"• банить, кикать и мутить нарушителей;\n"+
		"• выдавать предупреждения с настраиваемым лимитом;\n"+
		"• вести списки банов и мутов;\n"+
		"• назначать админов и модераторов с индивидуальными правами (только владелец группы);\n"+
		"• показывать правила, статистику чата и вашу личную статистику;\n"+
		"• принимать причину наказания свободным текстом при бане, муте и варне.\n\n"+
		"Команды работают через /, ! и +. Например: !мут 10 минут.\n"+
		"Все команды и синтаксис: /help", name)
}

// privateIntroMarkup builds the "add me as a full-rights admin" deep link.
// Telegram clients handle t.me/<bot>?startgroup&admin=<rights> by opening the
// group picker and promoting the bot with the requested rights after the owner
// confirms. The prompt is a request: the client shows a confirmation screen.
func privateIntroMarkup(self *tg.User) tg.ReplyMarkupClass {
	username := "irisgram_bot"
	if self != nil && strings.TrimSpace(self.Username) != "" {
		username = strings.TrimSpace(self.Username)
	}
	rights := strings.Join([]string{
		"change_info",
		"post_messages",
		"edit_messages",
		"delete_messages",
		"restrict_members",
		"invite_users",
		"pin_messages",
		"manage_topics",
		"promote_members",
		"manage_video_chats",
		"anonymous",
		"manage_chat",
		"post_stories",
		"edit_stories",
		"delete_stories",
		"manage_direct_messages",
		"manage_tags",
	}, "+")
	return &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{{
			Text: "➕ Добавить в группу (админ)",
			Type: &tg.InlineButtonTypeURL{
				URL: fmt.Sprintf("https://t.me/%s?startgroup=&admin=%s", username, rights),
			},
		}}},
	}}
}

func helpText() string {
	return "🛡 Ирис — модератор DeltaGram\n\n" +
		"Команды можно писать со слешем (/), восклицательным знаком (!) или плюсом (+):\n" +
		" /бан, !бан, +бан — это одна и та же команда.\n" +
		"Русские и английские названия работают одинаково.\n\n" +
		"🎯 КОГО НАКАЗАТЬ\n" +
		"• ответом на сообщение: !бан (ответьте на сообщение нарушителя);\n" +
		"• по ID DeltaGram: !бан 1780243200 или !бан id1780243200.\n" +
		"Юзернеймы и имена не принимаются — только числовой ID DeltaGram.\n" +
		"Узнать ID: ответьте на сообщение пользователя командой /айди.\n\n" +
		"⏱ ВРЕМЯ\n" +
		"30с / 30 секунд, 10м / 10 минут, 1ч / 1 час, 1д / 1 день, 1н / 1 неделя, 1мес.\n" +
		"0 или «навсегда» — бессрочно. Если время не указано, берётся из настроек.\n\n" +
		"🚫 БАН И МУТ\n" +
		"/бан [ID] [время] [причина] — забанить.\n" +
		"  Примеры: /бан 1780243200 1д флуд; !бан (ответом) навсегда;\n" +
		"/разбан [ID] — снять бан.\n" +
		"/кик [ID] [причина] — удалить из чата без бана.\n" +
		"/мут [ID] [время] [причина] — замутить.\n" +
		"  Примеры: !мут (ответом) 10 минут; /мут 1780243200 1ч флуд.\n" +
		"/размут [ID] — снять мут.\n\n" +
		"📝 ПРИЧИНА НАКАЗАНИЯ\n" +
		"После цели и времени можно написать любой текст — это причина.\n" +
		"Она сохраняется в записи и показывается в сообщении о наказании.\n" +
		"  /бан 1780243200 1д спам рекламой\n" +
		"  !мут (ответом) 30м нецензурная лексика\n" +
		"  /варн 1780243200 неуважение к участникам\n" +
		"Если время не указано, причина пишется сразу после цели.\n\n" +
		"⚠️ ПРЕДУПРЕЖДЕНИЯ\n" +
		"/варн [ID] [причина] — выдать предупреждение.\n" +
		"  Пример: !варн 1780243200 спам.\n" +
		"/снятьварн [ID] — снять последнее предупреждение.\n" +
		"/варны [ID] — показать предупреждения (без цели — свои).\n" +
		"/сбросварнов [ID] — сбросить все предупреждения.\n" +
		"При достижении лимита бот применит наказание из настроек.\n\n" +
		"📋 СПИСКИ И ИНФОРМАЦИЯ\n" +
		"/банлист — список забаненных.\n" +
		"/муты — список замьюченных.\n" +
		"/статистика — ваша личная статистика: сообщения, звёзды, подарки.\n" +
		"/статистика чат — статистика чата (для админов и модераторов).\n" +
		"/инфо [ID] — профиль и права пользователя.\n" +
		"/айди — ID пользователя из ответа (для модераторов и выше).\n" +
		"/id — ваши ID и ID чата.\n" +
		"/правила — правила чата.\n" +
		"/моиправа — какие права есть у вас.\n\n" +
		"👑 ТОЛЬКО ВЛАДЕЛЕЦ ЧАТА\n" +
		"/настройки — лимит варнов, длительность мута/бана, наказание.\n" +
		"/модер [ID] — назначить модератора (ответом тоже можно).\n" +
		"/размодер [ID] — снять модератора.\n" +
		"/модеры — список модераторов и их прав.\n" +
		"/админ [ID] — назначить админа бота (все права).\n" +
		"/разжаловать [ID] — снять админа бота.\n" +
		"/админы — список администрации.\n" +
		"/установитьправила <текст> — задать правила чата.\n\n" +
		"🧑‍⚖️ ПРАВА МОДЕРАТОРОВ\n" +
		"В «Настройки» → «Модераторы и их права» владелец может включать и выключать каждое право:\n" +
		"бан, разбан, кик, мут, размут, варн, снять варн, сброс варнов, списки и статистика.\n\n" +
		"Владелец и администраторы чата всегда имеют полные права. Модератор не может наказывать администраторов и других модераторов.\n\n" +
		"🔒 /help доступна только модераторам и выше."
}

func settingsText(s Settings) string {
	return fmt.Sprintf("⚙️ Настройки модерации\n\n"+
		"Лимит предупреждений: %d\n"+
		"Наказание при лимите: %s\n"+
		"Длительность мута: %s\n"+
		"Срок действия варна: %s\n"+
		"Длительность бана: %s\n"+
		"Правила чата: %s\n\n"+
		"Менять настройки может только владелец группы.",
		s.WarnLimit,
		punishLabel(s.WarnPunish),
		formatDuration(s.MuteSeconds),
		formatDuration(s.WarnExpirySeconds),
		formatDuration(s.BanSeconds),
		rulesState(s.Rules))
}

func rulesState(rules string) string {
	if strings.TrimSpace(rules) == "" {
		return "не заданы"
	}
	return "заданы"
}

func settingsMarkup(s Settings) tg.ReplyMarkupClass {
	rows := []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("−", "st:limit:-"),
			inlineButton(fmt.Sprintf("Лимит варнов: %d", s.WarnLimit), "st:limit:="),
			inlineButton("+", "st:limit:+"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("Мут: "+formatDuration(s.MuteSeconds), "st:mute:next"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("Срок варна: "+formatDuration(s.WarnExpirySeconds), "st:warntime:next"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("Наказание: "+punishLabel(s.WarnPunish), "st:punish:next"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("Бан: "+formatDuration(s.BanSeconds), "st:ban:next"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("🧑‍⚖️ Модераторы и их права", "st:mods:x"),
		}},
		{Buttons: []tg.KeyboardInlineButton{
			inlineButton("Сбросить", "st:reset:x"),
			inlineButton("Закрыть", "st:close:x"),
		}},
	}
	return &tg.ReplyInlineMarkup{Rows: rows}
}

func inlineButton(text, data string) tg.KeyboardInlineButton {
	return tg.KeyboardInlineButton{
		Text: text,
		Type: &tg.InlineButtonTypeCallback{Data: []byte(data)},
	}
}
