package bots

// This file implements the built-in operator gift-distribution bot
// (domain.GiftBotUserID).
//
// The bot is restricted to the operator accounts configured through
// TELESRV_GIFT_BOT_ADMIN_IDS. It shows the current star-gift catalog, lets the
// operator pick a gift and a per-user quantity, and then grants that gift to
// every ordinary DeltaGram account. Grants go through the admin service so each
// one is recorded with a deterministic command key and is safe to retry.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"telesrv/internal/domain"
)

const (
	giftBotStateCommand      = "gift_broadcast"
	giftBotStateStepQuantity = "quantity"
	giftBotMaxQuantity       = 10
	giftBotMaxCatalogButtons = 90
)

// giftBroadcaster is the slice of the star-gift/admin services the bot needs:
// the enabled catalog and a free, auditable grant to one user.
type giftBroadcaster interface {
	Catalog(ctx context.Context) ([]domain.StarGift, error)
	SendGift(ctx context.Context, userID, giftID int64, commandKey string) error
}

// giftRecipients enumerates the broadcast audience (ordinary, non-bot,
// non-deleted accounts).
type giftRecipients interface {
	RealUserIDs(ctx context.Context) ([]int64, error)
}

func (s *Service) giftBotAdmin(userID int64) bool {
	if s == nil || userID <= 0 {
		return false
	}
	_, ok := s.giftBotAdmins[userID]
	return ok
}

// NotifyOperators pushes an operator alert into every configured gift-bot
// admin chat. It is used for suspicious security events (runaway star mints,
// huge withdrawals) so the owner sees them in the same bot they already use,
// without polling the panel. Delivery is best-effort: an alert never fails the
// operation that triggered it, and the text is passed verbatim (callers send
// it in CAPS LOCK).
func (s *Service) NotifyOperators(ctx context.Context, text string) {
	if s == nil || s.messages == nil || strings.TrimSpace(text) == "" || len(s.giftBotAdmins) == 0 {
		return
	}
	for userID := range s.giftBotAdmins {
		if userID <= 0 || domain.IsSystemUserID(userID) {
			continue
		}
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{Text: text})
	}
}

// respondAsGiftBot answers one private message to the gift bot. Unknown
// commands and non-operators stay silent.
func (s *Service) respondAsGiftBot(userID int64, body string) {
	if !s.giftBotAdmin(userID) {
		return
	}
	mu := s.serviceBotReplyLock(domain.GiftBotUserID, userID)
	mu.Lock()
	defer mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	state, found, err := s.bots.GetBotChatState(ctx, domain.GiftBotUserID, userID)
	if err == nil && found && state.Command == giftBotStateCommand && state.Step == giftBotStateStepQuantity {
		s.giftBotCustomQuantity(ctx, userID, state, body)
		return
	}

	cmd, ok := parseBotCommand(strings.TrimSpace(body))
	if !ok {
		return
	}
	switch cmd {
	case "start", "gifts":
		s.giftBotCatalog(ctx, userID)
	case "help":
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{Text: giftBotHelpText()})
	default:
		// Unknown commands stay silent.
		return
	}
}

func giftBotHelpText() string {
	return "🎁 DeltaGram Gifts\n\n" +
		"Служебный бот для выдачи подарков всем пользователям DeltaGram.\n\n" +
		"/gifts — показать каталог подарков.\n" +
		"Выберите подарок, затем количество на каждого пользователя (1–10).\n" +
		"Подарки выдаются бесплатно и появляются в профиле получателя."
}

// giftBotCatalog lists the currently enabled gifts as inline buttons.
func (s *Service) giftBotCatalog(ctx context.Context, userID int64) {
	if s.giftBroadcaster == nil {
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
			Text: "⚙️ Каталог подарков недоступен. Попробуйте позже.",
		})
		return
	}
	gifts, err := s.giftBroadcaster.Catalog(ctx)
	if err != nil {
		s.log.Error("giftbot: load catalog", zap.Error(err))
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
			Text: "❌ Не удалось загрузить каталог подарков.",
		})
		return
	}
	rows := make([][]domain.MarkupButton, 0, len(gifts)+1)
	for _, gift := range gifts {
		if len(rows) >= giftBotMaxCatalogButtons {
			break
		}
		rows = append(rows, []domain.MarkupButton{{
			Type: domain.MarkupButtonCallback,
			Text: giftBotGiftButtonLabel(gift),
			Data: []byte("gb:g:" + strconv.FormatInt(gift.ID, 10)),
		}})
	}
	if len(rows) == 0 {
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
			Text: "Каталог подарков пуст.",
		})
		return
	}
	text := fmt.Sprintf("🎁 Подарки в каталоге: %d\n\nВыберите подарок, который нужно раздать всем пользователям DeltaGram.\nПосле выбора укажите количество на каждого получателя.", len(rows))
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
		Text:        text,
		ReplyMarkup: giftBotMarkup(rows),
	})
}

func giftBotGiftButtonLabel(gift domain.StarGift) string {
	title := strings.TrimSpace(gift.Title)
	if title == "" {
		title = "Подарок #" + strconv.FormatInt(gift.ID, 10)
	}
	label := fmt.Sprintf("🎁 %s — ⭐ %d", title, gift.Stars)
	if gift.SoldOut {
		label = "⚠️ " + label + " (распродан)"
	}
	return label
}

// giftBotQuantityMenu offers the per-user quantity presets and a free-form
// option.
func (s *Service) giftBotQuantityMenu(ctx context.Context, userID, giftID int64) {
	gift, ok := s.giftBotLookup(ctx, giftID)
	if !ok {
		s.giftBotCatalog(ctx, userID)
		return
	}
	recipients := 0
	if s.giftRecipients != nil {
		if ids, err := s.giftRecipients.RealUserIDs(ctx); err == nil {
			recipients = len(ids)
		}
	}
	label := giftBotGiftName(gift)
	rows := make([][]domain.MarkupButton, 0, 4)
	presetRow := make([]domain.MarkupButton, 0, 5)
	for _, qty := range []int{1, 2, 3, 5, 10} {
		presetRow = append(presetRow, domain.MarkupButton{
			Type: domain.MarkupButtonCallback,
			Text: strconv.Itoa(qty),
			Data: []byte(fmt.Sprintf("gb:q:%d:%d", giftID, qty)),
		})
	}
	rows = append(rows, presetRow)
	rows = append(rows, []domain.MarkupButton{
		{Type: domain.MarkupButtonCallback, Text: "✏️ Своё количество", Data: []byte(fmt.Sprintf("gb:q:%d:c", giftID))},
	})
	rows = append(rows, []domain.MarkupButton{
		{Type: domain.MarkupButtonCallback, Text: "⬅️ Назад к каталогу", Data: []byte("gb:menu")},
	})
	text := fmt.Sprintf("🎁 %s\n\nСколько подарить каждому пользователю?", label)
	if recipients > 0 {
		text += fmt.Sprintf("\nПолучателей: %d", recipients)
	}
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
		Text:        text,
		ReplyMarkup: giftBotMarkup(rows),
	})
}

// giftBotCustomPrompt stores the selected gift and asks for a number. The next
// text message is parsed as the quantity instead of a command.
func (s *Service) giftBotCustomPrompt(ctx context.Context, userID, giftID int64) {
	if err := s.bots.UpsertBotChatState(ctx, domain.BotChatState{
		BotUserID: domain.GiftBotUserID,
		UserID:    userID,
		Command:   giftBotStateCommand,
		Step:      giftBotStateStepQuantity,
		Draft:     map[string]string{"gift_id": strconv.FormatInt(giftID, 10)},
	}); err != nil {
		s.log.Error("giftbot: save quantity state", zap.Error(err))
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
			Text: "❌ Не удалось сохранить выбор. Попробуйте ещё раз.",
		})
		return
	}
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
		Text: fmt.Sprintf("Введите количество от 1 до %d на каждого пользователя.", giftBotMaxQuantity),
	})
}

func (s *Service) giftBotCustomQuantity(ctx context.Context, userID int64, state domain.BotChatState, body string) {
	qty, err := strconv.Atoi(strings.TrimSpace(body))
	if err != nil || qty < 1 || qty > giftBotMaxQuantity {
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
			Text: fmt.Sprintf("Введите целое число от 1 до %d.", giftBotMaxQuantity),
		})
		return
	}
	giftID, _ := strconv.ParseInt(strings.TrimSpace(state.Draft["gift_id"]), 10, 64)
	_ = s.bots.DeleteBotChatState(ctx, domain.GiftBotUserID, userID)
	if giftID <= 0 {
		s.giftBotCatalog(ctx, userID)
		return
	}
	s.giftBotConfirm(ctx, userID, giftID, qty)
}

// giftBotConfirm shows the final confirmation with the exact per-user quantity
// and total grant count.
func (s *Service) giftBotConfirm(ctx context.Context, userID, giftID int64, qty int) {
	gift, ok := s.giftBotLookup(ctx, giftID)
	if !ok {
		s.giftBotCatalog(ctx, userID)
		return
	}
	recipients := 0
	if s.giftRecipients != nil {
		if ids, err := s.giftRecipients.RealUserIDs(ctx); err == nil {
			recipients = len(ids)
		}
	}
	text := fmt.Sprintf("Проверьте рассылку:\n\n🎁 Подарок: %s\n👥 Получателей: %d\n📦 На каждого: %d\nΣ Всего подарков: %d\n\nРаздать всем пользователям DeltaGram?",
		giftBotGiftName(gift), recipients, qty, recipients*qty)
	rows := [][]domain.MarkupButton{
		{
			{Type: domain.MarkupButtonCallback, Text: "✅ Подтвердить", Data: []byte(fmt.Sprintf("gb:c:%d:%d", giftID, qty))},
			{Type: domain.MarkupButtonCallback, Text: "❌ Отмена", Data: []byte("gb:x")},
		},
	}
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
		Text:        text,
		ReplyMarkup: giftBotMarkup(rows),
	})
}

// giftBotBeginBroadcast claims the single broadcast slot and runs the fan-out
// on a detached goroutine so the callback returns immediately.
func (s *Service) giftBotBeginBroadcast(ctx context.Context, userID, giftID int64, qty int) (domain.BotCallbackAnswer, bool, error) {
	if qty < 1 || qty > giftBotMaxQuantity {
		return domain.BotCallbackAnswer{Message: "Некорректное количество.", Alert: true}, true, nil
	}
	if s.giftBroadcaster == nil || s.giftRecipients == nil {
		return domain.BotCallbackAnswer{Message: "Сервис подарков недоступен.", Alert: true}, true, nil
	}
	if !s.giftBotRunMu.TryLock() {
		return domain.BotCallbackAnswer{Message: "Рассылка уже выполняется. Дождитесь её завершения.", Alert: true}, true, nil
	}
	batch, err := randomInt64()
	if err != nil {
		s.giftBotRunMu.Unlock()
		return domain.BotCallbackAnswer{Message: "Не удалось запустить рассылку.", Alert: true}, true, nil
	}
	// The acknowledgement is written here; the slow fan-out is detached from the
	// short-lived callback RPC.
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{
		Text: fmt.Sprintf("🚀 Начинаю рассылку: %d подарк(ов) на каждого получателя. Пришлю итог, когда закончу.", qty),
	})
	go func() {
		defer s.giftBotRunMu.Unlock()
		runCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		s.runGiftBotBroadcast(runCtx, userID, giftID, qty, batch)
	}()
	return domain.BotCallbackAnswer{Message: "Рассылка запущена."}, true, nil
}

func (s *Service) runGiftBotBroadcast(ctx context.Context, adminID, giftID int64, qty int, batch int64) {
	giftName := "Подарок #" + strconv.FormatInt(giftID, 10)
	if gift, ok := s.giftBotLookup(ctx, giftID); ok {
		giftName = giftBotGiftName(gift)
	}
	recipients, err := s.giftRecipients.RealUserIDs(ctx)
	if err != nil {
		s.log.Error("giftbot: list recipients", zap.Error(err))
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, adminID, botReply{
			Text: "❌ Не удалось получить список пользователей. Рассылка не запущена.",
		})
		return
	}
	sent, failed := 0, 0
	for _, userID := range recipients {
		for idx := 0; idx < qty; idx++ {
			key := fmt.Sprintf("giftbot:%d:%d:%d:%d", batch, userID, giftID, idx)
			if err := s.giftBroadcaster.SendGift(ctx, userID, giftID, key); err != nil {
				failed++
				s.log.Warn("giftbot: send gift",
					zap.Int64("user_id", userID), zap.Int64("gift_id", giftID), zap.Error(err))
				continue
			}
			sent++
		}
	}
	lines := []string{
		"✅ Рассылка завершена",
		"",
		"🎁 Подарок: " + giftName,
		fmt.Sprintf("👥 Получателей: %d", len(recipients)),
		fmt.Sprintf("📦 На каждого: %d", qty),
		fmt.Sprintf("📨 Отправлено: %d", sent),
	}
	if failed > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ Ошибок: %d", failed))
	}
	s.sendServiceBotReply(ctx, domain.GiftBotUserID, adminID, botReply{Text: strings.Join(lines, "\n")})
}

// onGiftBotCallback routes inline-button clicks for the gift bot.
func (s *Service) onGiftBotCallback(ctx context.Context, query domain.BotCallbackQuery) (domain.BotCallbackAnswer, bool, error) {
	if !s.giftBotAdmin(query.UserID) {
		// Silence for non-operators: answer the click but show nothing.
		return domain.BotCallbackAnswer{}, true, nil
	}
	parts := strings.Split(string(query.Data), ":")
	if len(parts) < 2 || parts[0] != "gb" {
		return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
	}
	userID := query.UserID
	switch parts[1] {
	case "menu":
		s.giftBotCatalog(ctx, userID)
		return domain.BotCallbackAnswer{}, true, nil
	case "x":
		_ = s.bots.DeleteBotChatState(ctx, domain.GiftBotUserID, userID)
		s.sendServiceBotReply(ctx, domain.GiftBotUserID, userID, botReply{Text: "Отменено."})
		return domain.BotCallbackAnswer{}, true, nil
	case "g":
		if len(parts) != 3 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		giftID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || giftID <= 0 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		s.giftBotQuantityMenu(ctx, userID, giftID)
		return domain.BotCallbackAnswer{}, true, nil
	case "q":
		if len(parts) != 4 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		giftID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || giftID <= 0 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		if parts[3] == "c" {
			s.giftBotCustomPrompt(ctx, userID, giftID)
			return domain.BotCallbackAnswer{}, true, nil
		}
		qty, err := strconv.Atoi(parts[3])
		if err != nil || qty < 1 || qty > giftBotMaxQuantity {
			return domain.BotCallbackAnswer{Message: "Некорректное количество.", Alert: true}, true, nil
		}
		_ = s.bots.DeleteBotChatState(ctx, domain.GiftBotUserID, userID)
		s.giftBotConfirm(ctx, userID, giftID, qty)
		return domain.BotCallbackAnswer{}, true, nil
	case "c":
		if len(parts) != 4 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		giftID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || giftID <= 0 {
			return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
		}
		qty, err := strconv.Atoi(parts[3])
		if err != nil {
			return domain.BotCallbackAnswer{Message: "Некорректное количество.", Alert: true}, true, nil
		}
		return s.giftBotBeginBroadcast(ctx, userID, giftID, qty)
	default:
		return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
	}
}

func (s *Service) giftBotLookup(ctx context.Context, giftID int64) (domain.StarGift, bool) {
	if s == nil || s.giftBroadcaster == nil {
		return domain.StarGift{}, false
	}
	gifts, err := s.giftBroadcaster.Catalog(ctx)
	if err != nil {
		return domain.StarGift{}, false
	}
	for _, gift := range gifts {
		if gift.ID == giftID {
			return gift, true
		}
	}
	return domain.StarGift{}, false
}

func giftBotGiftName(gift domain.StarGift) string {
	title := strings.TrimSpace(gift.Title)
	if title == "" {
		title = "Подарок #" + strconv.FormatInt(gift.ID, 10)
	}
	return fmt.Sprintf("%s (⭐ %d)", title, gift.Stars)
}

func giftBotMarkup(rows [][]domain.MarkupButton) *domain.MessageReplyMarkup {
	return &domain.MessageReplyMarkup{
		Type:   domain.MessageReplyMarkupInline,
		Inline: rows,
	}
}
