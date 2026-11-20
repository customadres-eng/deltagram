package service

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"telesrv/cmd/casino/internal/domain"
	"telesrv/cmd/casino/internal/store/postgres"
)

// RoundService runs the four games: it opens bets, resolves outcomes from the
// provably-fair RNG, and settles wins and losses onto the casino balance. Every
// bet is idempotent on a client-supplied command id, so a retried request can
// never double-charge a player.
type RoundService struct {
	db           *postgres.DB
	chipsPerStar int64
	bank         *NftBank
}

// defaultCrashMaxMultiplier caps a crash cashout, matching the game engine's
// default ceiling.
const defaultCrashMaxMultiplier = 100

// NewRoundService builds the round runner.
func NewRoundService(db *postgres.DB, chipsPerStar int64) *RoundService {
	return &RoundService{db: db, chipsPerStar: chipsPerStar}
}

// SetNftBank wires the NFT prize pool so the upgrade game can award a real NFT
// on a win. It is optional: without it, upgrade pays chips only.
func (s *RoundService) SetNftBank(b *NftBank) { s.bank = b }

// PayoutPercent is the operator-configured win multiplier (100 = designed RTP).
// Exposed to the Mini App so the client can show the real numbers instead of a
// hardcoded 95%.
func (s *RoundService) PayoutPercent(ctx context.Context) int {
	return s.db.PayoutPercent(ctx)
}

// scalePayout applies the operator payout multiplier to a winning amount. A
// zero setting pays nothing, which settlement treats as a loss.
func scalePayout(payout int64, pct int) int64 {
	if payout <= 0 || pct <= 0 {
		return 0
	}
	return payout * int64(pct) / 100
}

// scaleMultiplier keeps the displayed multiplier consistent with the scaled
// payout (payout = stake * multiplier).
func scaleMultiplier(mult float64, pct int) float64 {
	if mult <= 0 {
		return 0
	}
	return mult * float64(pct) / 100
}

// Result is the outcome of a settled (or still-active) round, returned to the
// mini app. Payout is in chips; Multiplier is for display.
type Result struct {
	RoundID        int64          `json:"round_id"`
	Game           string         `json:"game"`
	Stake          int64          `json:"stake"`
	Payout         int64          `json:"payout"`
	Status         string         `json:"status"`
	Multiplier     float64        `json:"multiplier"`
	Outcome        map[string]any `json:"outcome,omitempty"`
	ServerSeed     string         `json:"server_seed,omitempty"`
	ServerSeedHash string         `json:"server_seed_hash,omitempty"`
	NftGiftRowID   int64          `json:"nft_gift_row_id,omitempty"`
	NftValue       int64          `json:"nft_value,omitempty"`
	StartedAt      int64          `json:"started_at,omitempty"`
	Balance        int64          `json:"balance"`
}

// PlaySlots resolves a single spin in one shot: debit stake, spin, settle.
//
// The engine is the 6x5 scatter-pays "Gates" slot: symbols pay by total count
// anywhere on the board, winning symbols tumble away and multiplier orbs boost
// the win. The whole cascade (initial board, every tumble and the orb total) is
// returned so the client can replay it frame by frame.
func (s *RoundService) PlaySlots(ctx context.Context, userID, stake int64, clientSeed, commandID string) (Result, error) {
	return s.playOnce(ctx, userID, "slots", stake, clientSeed, commandID, nil,
		func(rng *domain.RNG) (int64, map[string]any, float64) {
			spin := rng.GatesSpin()
			out := map[string]any{
				"initial":    spin.Initial,
				"grid":       spin.Grid,
				"tumbles":    spin.Tumbles,
				"orb_total":  spin.OrbTotal,
				"multiplier": spin.Multiplier,
			}
			return domain.Payout(stake, spin.Multiplier), out, spin.Multiplier
		})
}

// PlayUpgrade resolves a single-shot upgrade against a target multiplier.
func (s *RoundService) PlayUpgrade(ctx context.Context, userID, stake int64, target float64, clientSeed, commandID string) (Result, error) {
	if target <= 0 || target > 10000 {
		return Result{}, errors.New("target multiplier out of range")
	}
	chance := domain.UpgradeChance(target)
	return s.playOnce(ctx, userID, "upgrade", stake, clientSeed, commandID,
		map[string]any{"target": target, "chance": chance},
		func(rng *domain.RNG) (int64, map[string]any, float64) {
			won := rng.Upgrade(chance)
			if !won {
				return 0, map[string]any{"won": false, "multiplier": 0, "chance": chance}, 0
			}
			return domain.Payout(stake, target), map[string]any{"won": true, "multiplier": target, "chance": chance}, target
		})
}

// PlayUpgradeNft is PlayUpgrade but pays a real NFT from the prize pool on a
// win instead of chips. The awarded NFT is valued as close as possible to the
// stake times the target multiplier, and lands in the player's withdrawable
// casino inventory. If the pool has no affordable NFT, the win falls back to
// chips so a player is never short-changed.
func (s *RoundService) PlayUpgradeNft(ctx context.Context, userID, stake int64, target float64, clientSeed, commandID string) (Result, error) {
	if stake <= 0 {
		return Result{}, errors.New("stake must be positive")
	}
	if commandID == "" {
		return Result{}, errors.New("command id is required")
	}
	if target <= 0 || target > 10000 {
		return Result{}, errors.New("target multiplier out of range")
	}
	if s.bank == nil {
		return Result{}, errors.New("nft prize pool is not configured")
	}
	if existing, err := s.db.GetRoundByCommand(ctx, userID, commandID); err == nil {
		return s.resultFrom(ctx, existing), nil
	}
	chance := domain.UpgradeChance(target)
	seed, hash, err := domain.NewServerSeed()
	if err != nil {
		return Result{}, err
	}
	var res Result
	err = s.db.InTx(ctx, func(tx *postgres.DB) error {
		if _, err := tx.Apply(ctx, userID, -stake, "bet", commandID); err != nil {
			return err
		}
		round := postgres.Round{
			UserID: userID, Game: "upgrade", Stake: stake, CommandID: commandID,
			ServerSeed: seed[:], ServerSeedHash: hash[:], ClientSeed: clientSeed,
			Params: map[string]any{"target": target, "chance": chance, "nft": true},
			Status: postgres.RoundActive,
		}
		if err := tx.CreateRound(ctx, &round); err != nil {
			return err
		}
		won := domain.NewRNG(seed, clientSeed).Upgrade(chance)
		status := postgres.RoundLost
		pct := s.db.PayoutPercent(ctx)
		payout := int64(0)
		mult := 0.0
		outcome := map[string]any{"won": won, "chance": chance}
		if won {
			status = postgres.RoundWon
			payout = scalePayout(domain.Payout(stake, target), pct)
			mult = scaleMultiplier(target, pct)
			if payout <= 0 {
				status = postgres.RoundLost
				mult = 0
			} else {
				outcome["multiplier"] = mult
				// Award a real NFT worth the payout; fall back to chips if the pool
				// has nothing affordable at this price.
				giftRowID, value, err := s.bank.AwardNftTx(ctx, tx, userID, payout)
				switch {
				case err == nil:
					outcome["nft_gift_row_id"] = giftRowID
					outcome["nft_value"] = value
					res.NftGiftRowID, res.NftValue = giftRowID, value
				case errors.Is(err, postgres.ErrNotFound):
					if _, err := tx.Apply(ctx, userID, payout, "win", commandID+":win"); err != nil {
						return err
					}
					outcome["paid_in_chips"] = true
				default:
					return err
				}
			}
		}
		if err := tx.SettleRound(ctx, userID, round.ID, status, payout, outcome); err != nil {
			return err
		}
		balance, err := tx.Balance(ctx, userID)
		if err != nil {
			return err
		}
		res.RoundID, res.Game, res.Stake, res.Status = round.ID, "upgrade", stake, status
		res.Payout, res.Outcome = payout, outcome
		res.Multiplier = mult
		res.ServerSeed, res.ServerSeedHash = hex.EncodeToString(seed[:]), hex.EncodeToString(hash[:])
		res.Balance = balance
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// StartCrash opens a crash round: it commits the seed, debits the stake and
// draws the hidden crash point. The player cashes out before the point is
// reached (see CashoutCrash) or the round crashes on its own once the live
// multiplier passes it (see CrashState).
func (s *RoundService) StartCrash(ctx context.Context, userID, stake int64, clientSeed, commandID string) (Result, error) {
	res, err := s.openRound(ctx, userID, "crash", stake, clientSeed, commandID, nil)
	if err != nil {
		return Result{}, err
	}
	round, err := s.db.GetRound(ctx, userID, res.RoundID)
	if err != nil {
		return Result{}, err
	}
	crash := domain.NewRNG(seedOf(round), round.ClientSeed).Crash()
	// The crash point is committed to the round now but never returned to the
	// client until the round settles, so a player cannot know it in advance.
	if err := s.db.UpdateOutcome(ctx, userID, res.RoundID, map[string]any{"crash": crash}); err != nil {
		return Result{}, err
	}
	res.StartedAt = round.CreatedAt.UnixMilli()
	return res, nil
}

// CrashState reports a crash round's live state. While the round is active it
// returns the current multiplier; once the multiplier passes the hidden crash
// point the round settles itself as a loss and the crash point is revealed.
// The Mini App polls this so the rocket crashes in real time even if the player
// never presses cash out.
func (s *RoundService) CrashState(ctx context.Context, userID, roundID int64) (Result, error) {
	round, err := s.db.GetRound(ctx, userID, roundID)
	if err != nil {
		return Result{}, err
	}
	if round.Status != postgres.RoundActive {
		return s.resultFrom(ctx, round), nil
	}
	crash := crashFromOutcome(round.Outcome)
	if crash <= 0 {
		crash = domain.NewRNG(seedOf(round), round.ClientSeed).Crash()
	}
	current := domain.CrashMultiplierAt(time.Since(round.CreatedAt))
	if current >= crash {
		return s.settle(ctx, round, false, 0, map[string]any{"crash": crash, "cashed_at": 0})
	}
	balance, _ := s.db.Balance(ctx, userID)
	return Result{
		RoundID: roundID, Game: "crash", Stake: round.Stake,
		Status: postgres.RoundActive, Multiplier: current, Balance: balance,
		StartedAt: round.CreatedAt.UnixMilli(),
	}, nil
}

// CashoutCrash settles a crash round at the requested multiplier. The request
// is clamped to the current live multiplier, so a client cannot name a value
// its rocket has not reached yet; if the live multiplier already passed the
// hidden crash point the round is settled as a loss instead.
func (s *RoundService) CashoutCrash(ctx context.Context, userID, roundID int64, multiplier float64) (Result, error) {
	if multiplier < 1 {
		multiplier = 1
	}
	if multiplier > defaultCrashMaxMultiplier {
		multiplier = defaultCrashMaxMultiplier
	}
	round, err := s.db.GetRound(ctx, userID, roundID)
	if err != nil {
		return Result{}, err
	}
	if round.Status != postgres.RoundActive {
		return s.resultFrom(ctx, round), nil
	}
	crash := crashFromOutcome(round.Outcome)
	if crash <= 0 {
		crash = domain.NewRNG(seedOf(round), round.ClientSeed).Crash()
	}
	current := domain.CrashMultiplierAt(time.Since(round.CreatedAt))
	if current >= crash {
		return s.settle(ctx, round, false, 0, map[string]any{"crash": crash, "cashed_at": current})
	}
	if multiplier > current {
		multiplier = current
	}
	if multiplier < 1.01 {
		multiplier = 1.01
	}
	if multiplier >= crash {
		return s.settle(ctx, round, false, 0, map[string]any{"crash": crash, "cashed_at": multiplier})
	}
	return s.settle(ctx, round, true, multiplier, map[string]any{"crash": crash, "cashed_at": multiplier})
}

// crashFromOutcome safely reads the committed crash point from a persisted
// outcome (JSON numbers decode as float64).
func crashFromOutcome(outcome map[string]any) float64 {
	if v, ok := outcome["crash"].(float64); ok {
		return v
	}
	return 0
}

// StartMines opens a mines round with a chosen mine count. The board is drawn
// deterministically from the seed and stored server-side until settlement.
func (s *RoundService) StartMines(ctx context.Context, userID, stake int64, mines int, clientSeed, commandID string) (Result, error) {
	if mines < 1 || mines > 24 {
		return Result{}, errors.New("mines must be 1..24")
	}
	res, err := s.openRound(ctx, userID, "mines", stake, clientSeed, commandID, map[string]any{"mines": mines})
	if err != nil {
		return Result{}, err
	}
	// Draw the board now and store it in the round's outcome so each reveal is
	// a constant-time lookup against a fixed board.
	round, err := s.db.GetRound(ctx, userID, res.RoundID)
	if err != nil {
		return Result{}, err
	}
	board := domain.NewRNG(seedOf(round), round.ClientSeed).NewMinesBoard(domain.MinesGrid, mines)
	out := map[string]any{"grid": domain.MinesGrid, "mines": mines, "board": board.Cells, "revealed": []int{}}
	if err := s.db.UpdateOutcome(ctx, userID, res.RoundID, out); err != nil {
		return Result{}, err
	}
	res.Outcome = map[string]any{"grid": domain.MinesGrid, "mines": mines, "revealed": []int{}}
	return res, nil
}

// RevealMines flips one cell. A mine settles the round as a loss; a safe cell
// is recorded and the round stays active.
func (s *RoundService) RevealMines(ctx context.Context, userID, roundID int64, cell int) (Result, error) {
	if cell < 0 || cell >= domain.MinesGrid*domain.MinesGrid {
		return Result{}, errors.New("cell out of range")
	}
	round, err := s.db.GetRound(ctx, userID, roundID)
	if err != nil {
		return Result{}, err
	}
	if round.Status != postgres.RoundActive {
		return s.resultFrom(ctx, round), nil
	}
	board, revealed, err := minesState(round.Outcome)
	if err != nil {
		return Result{}, err
	}
	// Re-revealing an already open cell must never grow the multiplier: it would
	// let a player farm the same safe cell up to the maximum payout.
	for _, open := range revealed {
		if open == cell {
			balance, _ := s.db.Balance(ctx, userID)
			return Result{RoundID: roundID, Game: "mines", Stake: round.Stake, Status: postgres.RoundActive, Outcome: map[string]any{"revealed": revealed, "mine": false}, Balance: balance}, nil
		}
	}
	if board[cell] {
		out := map[string]any{"grid": domain.MinesGrid, "mines": round.Outcome["mines"], "board": board, "revealed": revealed, "mine_at": cell}
		return s.settle(ctx, round, false, 0, out)
	}
	revealed = append(revealed, cell)
	out := map[string]any{"grid": domain.MinesGrid, "mines": round.Outcome["mines"], "board": board, "revealed": revealed}
	if err := s.db.UpdateOutcome(ctx, userID, roundID, out); err != nil {
		return Result{}, err
	}
	balance, _ := s.db.Balance(ctx, userID)
	mines := minesFromOutcome(round.Outcome)
	mult := 0.0
	if mines > 0 {
		mult = domain.MinesMultiplier(domain.MinesGrid, mines, len(revealed))
	}
	return Result{RoundID: roundID, Game: "mines", Stake: round.Stake, Status: postgres.RoundActive, Multiplier: mult, Outcome: map[string]any{"revealed": revealed, "mine": false, "multiplier": mult}, Balance: balance}, nil
}

// CashoutMines settles a mines round at the current multiplier for the cells
// revealed so far.
func (s *RoundService) CashoutMines(ctx context.Context, userID, roundID int64) (Result, error) {
	round, err := s.db.GetRound(ctx, userID, roundID)
	if err != nil {
		return Result{}, err
	}
	_, revealed, err := minesState(round.Outcome)
	if err != nil {
		return Result{}, err
	}
	if len(revealed) == 0 {
		return Result{}, errors.New("reveal at least one cell before cashing out")
	}
	mines := minesFromOutcome(round.Outcome)
	if mines <= 0 {
		return Result{}, errors.New("mines count missing")
	}
	mult := domain.MinesMultiplier(domain.MinesGrid, mines, len(revealed))
	return s.settle(ctx, round, true, mult, map[string]any{"revealed": revealed, "multiplier": mult})
}

// minesFromOutcome safely reads the mine count from a persisted outcome. JSON
// round-trips numbers as float64, but an in-memory outcome may still hold int.
func minesFromOutcome(outcome map[string]any) int {
	switch v := outcome["mines"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

// playOnce runs a single-shot game (slots, upgrade) atomically.
func (s *RoundService) playOnce(ctx context.Context, userID int64, game string, stake int64, clientSeed, commandID string, params map[string]any, resolve func(*domain.RNG) (int64, map[string]any, float64)) (Result, error) {
	if stake <= 0 {
		return Result{}, errors.New("stake must be positive")
	}
	if commandID == "" {
		return Result{}, errors.New("command id is required")
	}
	if existing, err := s.db.GetRoundByCommand(ctx, userID, commandID); err == nil {
		return s.resultFrom(ctx, existing), nil
	}
	seed, hash, err := domain.NewServerSeed()
	if err != nil {
		return Result{}, err
	}
	var res Result
	err = s.db.InTx(ctx, func(tx *postgres.DB) error {
		if _, err := tx.Apply(ctx, userID, -stake, "bet", commandID); err != nil {
			return err
		}
		round := postgres.Round{
			UserID: userID, Game: game, Stake: stake, CommandID: commandID,
			ServerSeed: seed[:], ServerSeedHash: hash[:], ClientSeed: clientSeed,
			Params: params, Status: postgres.RoundActive,
		}
		if err := tx.CreateRound(ctx, &round); err != nil {
			return err
		}
		payout, outcome, mult := resolve(domain.NewRNG(seed, clientSeed))
		pct := s.db.PayoutPercent(ctx)
		mult = scaleMultiplier(mult, pct)
		payout = scalePayout(payout, pct)
		status := postgres.RoundWon
		if payout <= 0 {
			status = postgres.RoundLost
		}
		if err := tx.SettleRound(ctx, userID, round.ID, status, payout, outcome); err != nil {
			return err
		}
		if payout > 0 {
			if _, err := tx.Apply(ctx, userID, payout, "win", commandID+":win"); err != nil {
				return err
			}
		}
		balance, err := tx.Balance(ctx, userID)
		if err != nil {
			return err
		}
		res = Result{
			RoundID: round.ID, Game: game, Stake: stake, Payout: payout, Status: status,
			Multiplier: mult, Outcome: outcome,
			ServerSeed: hex.EncodeToString(seed[:]), ServerSeedHash: hex.EncodeToString(hash[:]),
			Balance: balance,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// openRound starts a multi-step round (crash, mines) and debits the stake.
func (s *RoundService) openRound(ctx context.Context, userID int64, game string, stake int64, clientSeed, commandID string, params map[string]any) (Result, error) {
	if stake <= 0 {
		return Result{}, errors.New("stake must be positive")
	}
	if commandID == "" {
		return Result{}, errors.New("command id is required")
	}
	if existing, err := s.db.GetRoundByCommand(ctx, userID, commandID); err == nil {
		return s.resultFrom(ctx, existing), nil
	}
	seed, hash, err := domain.NewServerSeed()
	if err != nil {
		return Result{}, err
	}
	var res Result
	err = s.db.InTx(ctx, func(tx *postgres.DB) error {
		if _, err := tx.Apply(ctx, userID, -stake, "bet", commandID); err != nil {
			return err
		}
		round := postgres.Round{
			UserID: userID, Game: game, Stake: stake, CommandID: commandID,
			ServerSeed: seed[:], ServerSeedHash: hash[:], ClientSeed: clientSeed,
			Params: params, Status: postgres.RoundActive,
		}
		if err := tx.CreateRound(ctx, &round); err != nil {
			return err
		}
		balance, err := tx.Balance(ctx, userID)
		if err != nil {
			return err
		}
		res = Result{
			RoundID: round.ID, Game: game, Stake: stake, Status: postgres.RoundActive,
			ServerSeedHash: hex.EncodeToString(hash[:]), Balance: balance,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// settle finalizes a multi-step round: mark won/lost, credit the payout, reveal
// the seed, and return the result.
func (s *RoundService) settle(ctx context.Context, round postgres.Round, won bool, multiplier float64, outcome map[string]any) (Result, error) {
	// A round can only be settled once. Without this guard a client could replay
	// a cashout (or a reveal that hit a mine) and be paid twice for one bet.
	if round.Status != postgres.RoundActive {
		return s.resultFrom(ctx, round), nil
	}
	pct := s.db.PayoutPercent(ctx)
	payout := int64(0)
	if won {
		payout = scalePayout(domain.Payout(round.Stake, multiplier), pct)
		multiplier = scaleMultiplier(multiplier, pct)
	}
	status := postgres.RoundLost
	if won && payout > 0 {
		status = postgres.RoundWon
	} else {
		// A suppressed payout (operator set 0%) is a loss for the player, not a
		// zero-value win.
		multiplier = 0
	}
	var res Result
	err := s.db.InTx(ctx, func(tx *postgres.DB) error {
		if err := tx.SettleRound(ctx, round.UserID, round.ID, status, payout, outcome); err != nil {
			return err
		}
		if payout > 0 {
			if _, err := tx.Apply(ctx, round.UserID, payout, "win", round.CommandID+":win"); err != nil {
				return err
			}
		}
		balance, err := tx.Balance(ctx, round.UserID)
		if err != nil {
			return err
		}
		res = Result{
			RoundID: round.ID, Game: round.Game, Stake: round.Stake, Payout: payout, Status: status,
			Multiplier: multiplier, Outcome: outcome,
			ServerSeed: hex.EncodeToString(round.ServerSeed), ServerSeedHash: hex.EncodeToString(round.ServerSeedHash),
			Balance: balance,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// resultFrom converts a persisted round into a Result for idempotent replays.
func (s *RoundService) resultFrom(ctx context.Context, round postgres.Round) Result {
	balance, _ := s.db.Balance(ctx, round.UserID)
	mult := 0.0
	if round.Stake > 0 && round.Payout > 0 {
		mult = float64(round.Payout) / float64(round.Stake)
	}
	return Result{
		RoundID: round.ID, Game: round.Game, Stake: round.Stake, Payout: round.Payout,
		Status: round.Status, Multiplier: mult, Outcome: round.Outcome,
		ServerSeed: hex.EncodeToString(round.ServerSeed), ServerSeedHash: hex.EncodeToString(round.ServerSeedHash),
		Balance: balance,
	}
}

// History returns the player's recent rounds as Results, newest first.
func (s *RoundService) History(ctx context.Context, userID int64, limit int) ([]Result, error) {
	rounds, err := s.db.ListRounds(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(rounds))
	for _, r := range rounds {
		out = append(out, s.resultFrom(ctx, r))
	}
	return out, nil
}

func seedOf(round postgres.Round) [32]byte {
	var seed [32]byte
	copy(seed[:], round.ServerSeed)
	return seed
}

// minesState decodes a mines outcome into its board and revealed cells.
func minesState(outcome map[string]any) ([]bool, []int, error) {
	rawBoard, ok := outcome["board"].([]any)
	if !ok {
		return nil, nil, errors.New("mines board missing")
	}
	board := make([]bool, len(rawBoard))
	for i, c := range rawBoard {
		b, ok := c.(bool)
		if !ok {
			return nil, nil, errors.New("mines board corrupt")
		}
		board[i] = b
	}
	revealed := []int{}
	if raw, ok := outcome["revealed"].([]any); ok {
		for _, r := range raw {
			if f, ok := r.(float64); ok {
				revealed = append(revealed, int(f))
			}
		}
	}
	return board, revealed, nil
}
