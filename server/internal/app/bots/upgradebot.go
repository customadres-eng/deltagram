package bots

// This file implements the built-in /upgrade bot (domain.UpgradeBotUserID).
//
// The bot runs inside the server process -- like @verifybot -- because the
// upgrade sweep must commit through the star-gift service's transaction
// coordinator. It has no MTProto session of its own and answers private
// messages through messages.Service's BotResponder hook.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"telesrv/internal/domain"
)

// respondAsUpgrade answers one private message to the /upgrade bot. The caller
// dispatches it on its own goroutine; the per-user reply lock keeps two rapid
// commands from starting overlapping sweeps.
func (s *Service) respondAsUpgrade(userID int64, body string) {
	mu := s.serviceBotReplyLock(domain.UpgradeBotUserID, userID)
	mu.Lock()
	defer mu.Unlock()

	// Upgrading several gifts is a chain of transactions, so the context is
	// detached from the already-returned RPC and gets a generous deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch upgradeCommand(body) {
	case "upgrade":
		s.sendServiceBotReply(ctx, domain.UpgradeBotUserID, userID, s.upgradeAllReply(ctx, userID))
	case "help":
		s.sendServiceBotReply(ctx, domain.UpgradeBotUserID, userID, botReply{Text: upgradeBotIntroText()})
	default:
		// Unknown commands stay silent.
		return
	}
}

// upgradeCommand normalises /upgrade and its aliases. /start and /help map to
// "help" so they still show the intro; every other command is ignored.
func upgradeCommand(body string) string {
	fields := strings.Fields(strings.TrimSpace(body))
	if len(fields) == 0 {
		return ""
	}
	head := fields[0]
	if cut := strings.IndexByte(head, '@'); cut >= 0 {
		head = head[:cut]
	}
	head = strings.ToLower(strings.TrimLeft(head, "/!+"))
	switch head {
	case "upgrade", "апгрейд", "улучшить":
		return "upgrade"
	case "start", "help", "старт", "помощь", "команды":
		return "help"
	}
	return ""
}

func upgradeBotIntroText() string {
	return "🎁 DeltaGram Upgrade\n\n" +
		"Отправьте /upgrade — я улучшу все неулучшенные подарки в вашем профиле.\n\n" +
		"• Звёзды списываются с вашего баланса по текущей цене улучшения.\n" +
		"• Подарки, для которых улучшение недоступно или распродано, пропускаются.\n" +
		"• Если звёзд не хватит, я остановлюсь и покажу, сколько успел улучшить."
}

func (s *Service) upgradeAllReply(ctx context.Context, userID int64) botReply {
	if s == nil || s.giftUpgrader == nil {
		return botReply{Text: "⚙️ Сервис улучшения подарков пока не подключён. Попробуйте позже."}
	}
	result, err := s.giftUpgrader.UpgradeAllOwned(ctx, userID, int(s.now().Unix()))
	if err != nil {
		return botReply{Text: "❌ Не удалось улучшить подарки. Попробуйте ещё раз позже."}
	}
	if result.Upgraded == 0 {
		switch result.StopReason {
		case "insufficient_stars":
			return botReply{Text: "⭐ Недостаточно звёзд для улучшения подарков."}
		default:
			if result.Skipped > 0 {
				return botReply{Text: "✅ Все ваши подарки уже улучшены или недоступны для улучшения."}
			}
			return botReply{Text: "🎁 В вашем профиле нет подарков, которые можно улучшить."}
		}
	}

	lines := []string{
		"✅ Готово! Улучшено подарков: " + fmt.Sprintf("%d", result.Upgraded),
		"💫 Потрачено звёзд: " + fmt.Sprintf("%d", result.SpentStars),
	}
	if result.BalanceKnown {
		lines = append(lines, fmt.Sprintf("⭐ Остаток звёзд: %d", result.Balance))
	}
	if result.StopReason == "insufficient_stars" {
		lines = append(lines, "", "⚠️ Звёзд не хватило на остальные подарки.")
	}
	if result.Skipped > 0 {
		lines = append(lines, fmt.Sprintf("↩️ Пропущено недоступных: %d", result.Skipped))
	}
	if result.Failed > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ Не удалось улучшить: %d", result.Failed))
	}
	return botReply{Text: strings.Join(lines, "\n")}
}
