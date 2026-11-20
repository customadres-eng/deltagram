package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"telesrv/internal/botdata"
)

const challengeTTL = 10 * time.Minute

type challenge struct {
	ID            int64
	ChatID        int64
	Kind          string
	ChallengerID  int64
	TargetID      int64
	ChallengerTag string
	TargetTag     string
	Expires       time.Time
}

// challengeStore keeps pending battles in memory. Losing them on restart is
// acceptable: a challenge is a 10-minute, single-use handshake, not durable
// state.
type challengeStore struct {
	mu    sync.Mutex
	items map[int64]*challenge
	now   func() time.Time
}

func newChallengeStore() *challengeStore {
	return &challengeStore{items: map[int64]*challenge{}, now: time.Now}
}

func (s *challengeStore) create(chatID int64, kind string, challengerID, targetID int64, challengerTag, targetTag string) *challenge {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	ch := &challenge{
		ID:            randomID(),
		ChatID:        chatID,
		Kind:          kind,
		ChallengerID:  challengerID,
		TargetID:      targetID,
		ChallengerTag: challengerTag,
		TargetTag:     targetTag,
		Expires:       now.Add(challengeTTL),
	}
	// One pending challenge per chat keeps the buttons unambiguous.
	for id, existing := range s.items {
		if existing.ChatID == chatID {
			delete(s.items, id)
		}
	}
	s.items[ch.ID] = ch
	return ch
}

func (s *challengeStore) get(token string) (*challenge, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(token), 10, 64)
	if err != nil {
		return nil, false
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	ch, ok := s.items[id]
	return ch, ok
}

func (s *challengeStore) remove(id int64) {
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}

func (s *challengeStore) pruneLocked(now time.Time) {
	for id, ch := range s.items {
		if !ch.Expires.After(now) {
			delete(s.items, id)
		}
	}
}

type mogVerdict struct {
	StarsWinner int // -1 challenger, 1 target, 0 tie
	BlackWinner int
	A           botdata.MogStats
	B           botdata.MogStats
	AWins       int
	BWins       int
	Winner      int // -1 challenger, 1 target, 0 draw
}

// compareMog scores exactly two categories: Stars and black-background gifts.
// Other profile gifts are deliberately ignored -- counting them would reward
// volume over the collectible black-backdrop line the battle is about.
func compareMog(a, b botdata.MogStats) mogVerdict {
	v := mogVerdict{A: a, B: b}
	v.StarsWinner = compareInts(a.Stars, b.Stars)
	v.BlackWinner = compareInts(a.Gifts.BlackBackdrop, b.Gifts.BlackBackdrop)
	for _, w := range []int{v.StarsWinner, v.BlackWinner} {
		switch w {
		case -1:
			v.AWins++
		case 1:
			v.BWins++
		}
	}
	switch {
	case v.AWins > v.BWins:
		v.Winner = -1
	case v.BWins > v.AWins:
		v.Winner = 1
	}
	return v
}

func compareInts(a, b int64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

// sendBattleResult publishes the full, open breakdown in the chat.
func (b *Bot) sendBattleResult(ctx context.Context, ch *challenge) {
	if b.data == nil {
		b.sendTo(ctx, ch.ChatID, ch.Kind, "⚙️ Не могу получить данные профилей: сервис статистики недоступен.", 0, nil)
		return
	}
	a, errA := b.data.Mog(ctx, ch.ChallengerID)
	if errA != nil {
		b.sendTo(ctx, ch.ChatID, ch.Kind, "❌ Не удалось получить статистику первого участника.", 0, nil)
		return
	}
	bb, errB := b.data.Mog(ctx, ch.TargetID)
	if errB != nil {
		b.sendTo(ctx, ch.ChatID, ch.Kind, "❌ Не удалось получить статистику второго участника.", 0, nil)
		return
	}
	v := compareMog(a, bb)
	lines := []string{
		"⚔️ МОГ-БАТЛ ЗАВЕРШЁН",
		"",
		ch.ChallengerTag + " vs " + ch.TargetTag,
		"",
		"💫 Звёзды: " + formatCount(a.Stars) + " — " + formatCount(bb.Stars) + verdictSuffix(v.StarsWinner, ch),
		"🖤 Подарки на чёрном фоне: " + formatCount(a.Gifts.BlackBackdrop) + " — " + formatCount(bb.Gifts.BlackBackdrop) + verdictSuffix(v.BlackWinner, ch),
		"",
		fmt.Sprintf("📋 Счёт по категориям: %s %d : %d %s", ch.ChallengerTag, v.AWins, v.BWins, ch.TargetTag),
		"",
		whyText(v, ch),
	}
	switch v.Winner {
	case -1:
		lines = append(lines, "", fmt.Sprintf("🏆 %s могнул %s (%d:%d).", ch.ChallengerTag, ch.TargetTag, v.AWins, v.BWins))
	case 1:
		lines = append(lines, "", fmt.Sprintf("🏆 %s могнул %s (%d:%d).", ch.TargetTag, ch.ChallengerTag, v.BWins, v.AWins))
	default:
		lines = append(lines, "", fmt.Sprintf("🤝 Ничья (%d:%d) — никто никого не могнул.", v.AWins, v.BWins))
	}
	b.sendTo(ctx, ch.ChatID, ch.Kind, strings.Join(lines, "\n"), 0, nil)
}

// whyText explains the verdict in plain language: which categories each side
// won and what the score means. It is the transparency layer the players asked
// for -- no hidden "formula" beyond the printed numbers.
func whyText(v mogVerdict, ch *challenge) string {
	const scoring = "В зачёт идут только звёзды и подарки на чёрном фоне — обычные подарки не учитываются."
	switch v.Winner {
	case -1:
		return fmt.Sprintf("Почему: %s оказался сильнее по %s. %s",
			ch.ChallengerTag, joinRussian(categoryNames(v, true)), scoring)
	case 1:
		return fmt.Sprintf("Почему: %s оказался сильнее по %s. %s",
			ch.TargetTag, joinRussian(categoryNames(v, false)), scoring)
	default:
		if v.AWins == 0 && v.BWins == 0 {
			return "Почему: у обоих одинаковые звёзды и одинаковое количество подарков на чёрном фоне. " + scoring
		}
		return fmt.Sprintf("Почему: %s сильнее по %s, а %s — по %s. Счёт по категориям равный, поэтому победителя нет. %s",
			ch.ChallengerTag, joinRussian(categoryNames(v, true)),
			ch.TargetTag, joinRussian(categoryNames(v, false)), scoring)
	}
}

func categoryNames(v mogVerdict, challenger bool) []string {
	want := 1
	if challenger {
		want = -1
	}
	out := make([]string, 0, 2)
	if v.StarsWinner == want {
		out = append(out, "звёздам")
	}
	if v.BlackWinner == want {
		out = append(out, "подаркам на чёрном фоне")
	}
	return out
}

func joinRussian(parts []string) string {
	switch len(parts) {
	case 0:
		return "ни одному показателю"
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " и " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " и " + parts[len(parts)-1]
	}
}

func verdictSuffix(winner int, ch *challenge) string {
	switch winner {
	case -1:
		return " → " + ch.ChallengerTag
	case 1:
		return " → " + ch.TargetTag
	default:
		return " → равенство"
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
