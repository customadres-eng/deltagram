package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"telesrv/internal/admin"
)

// Casino economy surface. Everything here reads the casino's own tables in the
// shared telesrv database: the panel owns no casino state, it only reads it and
// appends the operator audit trail when the payout multiplier changes.

// CasinoSettingsRow is the single casino_settings row (id = 1).
type CasinoSettingsRow struct {
	PayoutPercent int       `json:"payout_percent"`
	RTPPercent    float64   `json:"rtp_percent"`
	UpdatedAt     time.Time `json:"updated_at"`
	UpdatedBy     string    `json:"updated_by"`
}

type CasinoTotals struct {
	Players          int64   `json:"players"`
	Rounds           int64   `json:"rounds"`
	DepositsChips    int64   `json:"deposits_chips"`
	WithdrawalsChips int64   `json:"withdrawals_chips"`
	BetsChips        int64   `json:"bets_chips"`
	WinsChips        int64   `json:"wins_chips"`
	HouseChips       int64   `json:"house_chips"`
	ChipsLiability   int64   `json:"chips_liability"`
	StarsDeposited   int64   `json:"stars_deposited"`
	StarsWithdrawn   int64   `json:"stars_withdrawn"`
	ActiveLast24h    int64   `json:"active_last_24h"`
	RTPPercent       float64 `json:"rtp_percent"`
}

type CasinoGameStat struct {
	Game      string  `json:"game"`
	Rounds    int64   `json:"rounds"`
	Won       int64   `json:"won"`
	Stake     int64   `json:"stake"`
	Payout    int64   `json:"payout"`
	RTP       float64 `json:"rtp"`
	MaxPayout int64   `json:"max_payout"`
}

type CasinoUserStat struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	FirstName   string    `json:"first_name"`
	Balance     int64     `json:"balance"`
	Deposits    int64     `json:"deposits"`
	Withdrawals int64     `json:"withdrawals"`
	Bets        int64     `json:"bets"`
	Wins        int64     `json:"wins"`
	Net         int64     `json:"net"`
	Rounds      int64     `json:"rounds"`
	LastAction  time.Time `json:"last_action"`
}

type CasinoDailyStat struct {
	Day         string `json:"day"`
	Deposits    int64  `json:"deposits"`
	Withdrawals int64  `json:"withdrawals"`
	Bets        int64  `json:"bets"`
	Wins        int64  `json:"wins"`
}

type CasinoBigWin struct {
	RoundID    int64     `json:"round_id"`
	UserID     int64     `json:"user_id"`
	Username   string    `json:"username"`
	FirstName  string    `json:"first_name"`
	Game       string    `json:"game"`
	Stake      int64     `json:"stake"`
	Payout     int64     `json:"payout"`
	Multiplier float64   `json:"multiplier"`
	At         time.Time `json:"at"`
}

type CasinoStats struct {
	Settings      CasinoSettingsRow `json:"settings"`
	ChipsPerStar  int64             `json:"chips_per_star"`
	Totals        CasinoTotals      `json:"totals"`
	Games         []CasinoGameStat  `json:"games"`
	TopDepositors []CasinoUserStat  `json:"top_depositors"`
	TopWinners    []CasinoUserStat  `json:"top_winners"`
	TopLosers     []CasinoUserStat  `json:"top_losers"`
	TopBettors    []CasinoUserStat  `json:"top_bettors"`
	Users         []CasinoUserStat  `json:"users"`
	Daily         []CasinoDailyStat `json:"daily"`
	BigWins       []CasinoBigWin    `json:"big_wins"`
}

// CasinoSettings reads the economy row, falling back to the designed 100% when
// the casino has not created the table yet.
func (s *readStore) CasinoSettings(ctx context.Context) (CasinoSettingsRow, error) {
	out := CasinoSettingsRow{PayoutPercent: 100, RTPPercent: 95}
	err := s.pool.QueryRow(ctx, `
		SELECT payout_percent, updated_at, updated_by FROM casino_settings WHERE id = 1`).
		Scan(&out.PayoutPercent, &out.UpdatedAt, &out.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		// Missing table: the casino has never run its migrations on this host.
		if pgErrIsUndefinedTable(err) {
			return out, nil
		}
		return out, err
	}
	out.RTPPercent = 95 * float64(out.PayoutPercent) / 100
	return out, nil
}

// CasinoStats aggregates the whole casino: totals, per-game RTP, leaderboards,
// a bounded all-users table, a 30-day activity series and the biggest single
// wins.
func (s *readStore) CasinoStats(ctx context.Context, chipsPerStar int64) (CasinoStats, error) {
	if chipsPerStar <= 0 {
		chipsPerStar = 100
	}
	settings, err := s.CasinoSettings(ctx)
	if err != nil {
		return CasinoStats{}, err
	}
	out := CasinoStats{Settings: settings, ChipsPerStar: chipsPerStar}

	users, err := s.casinoUsers(ctx)
	if err != nil {
		return CasinoStats{}, err
	}
	out.Users = users

	// Totals derive from the same per-user rows to keep the numbers consistent
	// with the table the operator sees.
	var bets, wins, deposits, withdrawals int64
	for _, u := range users {
		bets += u.Bets
		wins += u.Wins
		deposits += u.Deposits
		withdrawals += u.Withdrawals
	}
	out.Totals.Players = int64(len(users))
	out.Totals.DepositsChips = deposits
	out.Totals.WithdrawalsChips = withdrawals
	out.Totals.BetsChips = bets
	out.Totals.WinsChips = wins
	out.Totals.HouseChips = bets - wins
	out.Totals.StarsDeposited = deposits / chipsPerStar
	out.Totals.StarsWithdrawn = withdrawals / chipsPerStar
	if bets > 0 {
		out.Totals.RTPPercent = float64(wins) * 100 / float64(bets)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM casino_rounds),
			(SELECT coalesce(sum(balance), 0) FROM casino_accounts),
			(SELECT count(DISTINCT user_id) FROM casino_ledger WHERE created_at >= now() - interval '24 hours')
	`).Scan(&out.Totals.Rounds, &out.Totals.ChipsLiability, &out.Totals.ActiveLast24h); err != nil {
		return CasinoStats{}, err
	}

	games, err := s.casinoGames(ctx)
	if err != nil {
		return CasinoStats{}, err
	}
	out.Games = games

	daily, err := s.casinoDaily(ctx)
	if err != nil {
		return CasinoStats{}, err
	}
	out.Daily = daily

	bigWins, err := s.casinoBigWins(ctx)
	if err != nil {
		return CasinoStats{}, err
	}
	out.BigWins = bigWins

	out.TopDepositors = topBy(users, func(u CasinoUserStat) int64 { return u.Deposits }, 20)
	out.TopBettors = topBy(users, func(u CasinoUserStat) int64 { return u.Bets }, 20)
	out.TopWinners = topBy(users, func(u CasinoUserStat) int64 { return u.Wins }, 20)
	out.TopLosers = bottomBy(users, func(u CasinoUserStat) int64 { return u.Net }, 20)
	return out, nil
}

func (s *readStore) casinoUsers(ctx context.Context) ([]CasinoUserStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.user_id,
		       coalesce(u.username, ''),
		       coalesce(u.first_name, ''),
		       coalesce(a.balance, 0),
		       coalesce(sum(CASE WHEN l.kind = 'deposit' THEN l.amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN l.kind = 'withdraw' THEN -l.amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN l.kind = 'bet' THEN -l.amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN l.kind = 'win' THEN l.amount ELSE 0 END), 0),
		       coalesce(max(l.created_at), now())
		FROM casino_ledger l
		LEFT JOIN public.users u ON u.id = l.user_id
		LEFT JOIN casino_accounts a ON a.user_id = l.user_id
		GROUP BY l.user_id, u.username, u.first_name, a.balance
		ORDER BY 7 DESC
		LIMIT 500`)
	if err != nil {
		if pgErrIsUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]CasinoUserStat, 0, 64)
	for rows.Next() {
		var u CasinoUserStat
		if err := rows.Scan(&u.UserID, &u.Username, &u.FirstName, &u.Balance,
			&u.Deposits, &u.Withdrawals, &u.Bets, &u.Wins, &u.LastAction); err != nil {
			return nil, err
		}
		u.Net = u.Wins - u.Bets
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rounds, err := s.casinoRoundsPerUser(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Rounds = rounds[out[i].UserID]
	}
	return out, nil
}

func (s *readStore) casinoRoundsPerUser(ctx context.Context) (map[int64]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id, count(*) FROM casino_rounds GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]int64, 64)
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (s *readStore) casinoGames(ctx context.Context) ([]CasinoGameStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT game,
		       count(*),
		       count(*) FILTER (WHERE status = 'won'),
		       coalesce(sum(stake), 0),
		       coalesce(sum(payout), 0),
		       coalesce(max(payout), 0)
		FROM casino_rounds
		GROUP BY game
		ORDER BY game`)
	if err != nil {
		if pgErrIsUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]CasinoGameStat, 0, 6)
	for rows.Next() {
		var g CasinoGameStat
		if err := rows.Scan(&g.Game, &g.Rounds, &g.Won, &g.Stake, &g.Payout, &g.MaxPayout); err != nil {
			return nil, err
		}
		if g.Stake > 0 {
			g.RTP = float64(g.Payout) * 100 / float64(g.Stake)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *readStore) casinoDaily(ctx context.Context) ([]CasinoDailyStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT to_char(created_at::date, 'YYYY-MM-DD'),
		       coalesce(sum(CASE WHEN kind = 'deposit' THEN amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN kind = 'withdraw' THEN -amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN kind = 'bet' THEN -amount ELSE 0 END), 0),
		       coalesce(sum(CASE WHEN kind = 'win' THEN amount ELSE 0 END), 0)
		FROM casino_ledger
		WHERE created_at >= date_trunc('day', now()) - interval '29 days'
		GROUP BY 1
		ORDER BY 1`)
	if err != nil {
		if pgErrIsUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]CasinoDailyStat, 0, 30)
	for rows.Next() {
		var d CasinoDailyStat
		if err := rows.Scan(&d.Day, &d.Deposits, &d.Withdrawals, &d.Bets, &d.Wins); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *readStore) casinoBigWins(ctx context.Context) ([]CasinoBigWin, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.user_id, coalesce(u.username, ''), coalesce(u.first_name, ''),
		       r.game, r.stake, r.payout,
		       CASE WHEN r.stake > 0 THEN r.payout::float8 / r.stake ELSE 0 END,
		       coalesce(r.settled_at, r.created_at)
		FROM casino_rounds r
		LEFT JOIN public.users u ON u.id = r.user_id
		WHERE r.status = 'won' AND r.payout > 0
		ORDER BY r.payout DESC
		LIMIT 20`)
	if err != nil {
		if pgErrIsUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]CasinoBigWin, 0, 20)
	for rows.Next() {
		var w CasinoBigWin
		if err := rows.Scan(&w.RoundID, &w.UserID, &w.Username, &w.FirstName, &w.Game,
			&w.Stake, &w.Payout, &w.Multiplier, &w.At); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func topBy(users []CasinoUserStat, key func(CasinoUserStat) int64, limit int) []CasinoUserStat {
	out := make([]CasinoUserStat, 0, limit)
	for _, u := range users {
		if key(u) <= 0 {
			continue
		}
		out = append(out, u)
	}
	sortUserStats(out, key, true)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func bottomBy(users []CasinoUserStat, key func(CasinoUserStat) int64, limit int) []CasinoUserStat {
	out := make([]CasinoUserStat, 0, limit)
	for _, u := range users {
		if key(u) >= 0 {
			continue
		}
		out = append(out, u)
	}
	sortUserStats(out, key, false)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortUserStats(rows []CasinoUserStat, key func(CasinoUserStat) int64, desc bool) {
	// Insertion sort keeps the dependency list tiny and the lists are <= 500.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			left, right := key(rows[j-1]), key(rows[j])
			if (desc && left >= right) || (!desc && left <= right) {
				break
			}
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
}

func pgErrIsUndefinedTable(err error) bool {
	return err != nil && (containsSQLState(err, "42P01") || containsSQLState(err, "42703"))
}

func containsSQLState(err error, state string) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == state
	}
	return false
}

// --- HTTP surface -----------------------------------------------------------

func (s *server) handleCasinoStatsAPI(w http.ResponseWriter, r *http.Request) {
	stats, err := s.read.CasinoStats(r.Context(), s.cfg.CasinoChipsPerStar)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *server) handleCasinoSettingsAPI(w http.ResponseWriter, r *http.Request) {
	settings, err := s.read.CasinoSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

type setCasinoPayoutRequest struct {
	CommandID     string `json:"command_id"`
	Reason        string `json:"reason"`
	Confirm       bool   `json:"confirm"`
	PayoutPercent int    `json:"payout_percent"`
}

// handleSetCasinoPayoutAPI changes the global win multiplier. It is a command
// like every other panel action: dry run first, admin_commands row, audit row,
// and the mutation in the same transaction.
func (s *server) handleSetCasinoPayoutAPI(w http.ResponseWriter, r *http.Request) {
	var body setCasinoPayoutRequest
	if !decodeAction(w, r, &body) {
		return
	}
	meta := s.commandMetaFromAPI(r, body.CommandID, body.Reason, body.Confirm, "casino-payout")
	const action = "casino.set-payout"
	params := map[string]any{"payout_percent": body.PayoutPercent}
	pct := body.PayoutPercent

	fn := func(ctx context.Context, tx pgx.Tx) (admin.CommandResult, error) {
		if pct < 0 || pct > 500 {
			return admin.CommandResult{}, fmt.Errorf("payout_percent must be between 0 and 500")
		}
		var previous int
		err := tx.QueryRow(ctx, `SELECT payout_percent FROM casino_settings WHERE id = 1 FOR UPDATE`).Scan(&previous)
		if errors.Is(err, pgx.ErrNoRows) {
			previous = 100
		} else if err != nil {
			return admin.CommandResult{}, err
		}
		details := map[string]any{
			"previous_payout_percent": previous,
			"payout_percent":          pct,
			"effective_rtp_percent":   95 * float64(pct) / 100,
		}
		if meta.DryRun {
			return admin.CommandResult{Status: "ok", DryRun: true, Message: "dry-run completed", Details: details}, nil
		}
		if previous == pct {
			details["unchanged"] = true
			return admin.CommandResult{Status: "ok", Message: "casino payout unchanged", Details: details}, nil
		}
		_, err = tx.Exec(ctx, `
			UPDATE casino_settings SET payout_percent = $1, updated_at = now(), updated_by = $2 WHERE id = 1`,
			pct, meta.Actor)
		if err != nil {
			return admin.CommandResult{}, err
		}
		return admin.CommandResult{Status: "ok", Message: "casino payout updated", Details: details}, nil
	}

	result, err := s.runOperatorCommand(r.Context(), meta, action, params, "", fn)
	writeCommandResultAPI(w, result, err)
}
