package domain

// Mines is the "reveal safe cells" game. A square grid hides a chosen number of
// mines; the player reveals cells one at a time and can cash out after each
// safe reveal. Revealing a mine loses the stake.
//
// RTP 95%: after k safe reveals out of a N-cell grid holding M mines, the
// probability of having survived this far is
//
//	p(k) = (N-M)/N * (N-M-1)/(N-1) * ... * (N-M-k+1)/(N-k+1)
//
// The fair (zero-edge) payout is 1/p(k). We scale it by (1 - edge) once, so a
// player who reveals exactly k cells and cashes out has expected return
// p(k) * (1-edge)/p(k) = 1-edge = 95% for every k.

const minesHouseEdge = 0.05

// MinesGrid is the side length of the default square board (5x5).
const MinesGrid = 5

// MinesBoard is a fixed arrangement of mines, indexed row-major: cell i is a
// mine when Cells[i] is true. The board is drawn with the round's RNG so the
// player can reproduce it from the seeds.
type MinesBoard struct {
	Grid  int    // side length; total cells = Grid*Grid
	Mines int    // number of mines placed
	Cells []bool // length Grid*Grid; true == mine
}

// NewMinesBoard places exactly mines mines on a grid*grid board using a
// Fisher-Yates shuffle driven by the RNG. It panics on an invalid mine count,
// mirroring a programmer error that must be caught before serving players.
func (r *RNG) NewMinesBoard(grid, mines int) MinesBoard {
	total := grid * grid
	if mines < 1 || mines > total-1 {
		panic("domain: invalid mines count")
	}
	// Index the cells 0..total-1, shuffle, and mark the first `mines` as mines.
	order := make([]int, total)
	for i := range order {
		order[i] = i
	}
	for i := total - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	board := MinesBoard{Grid: grid, Mines: mines, Cells: make([]bool, total)}
	for i := 0; i < mines; i++ {
		board.Cells[order[i]] = true
	}
	return board
}

// MinesMultiplier returns the payout multiplier after revealing `revealed` safe
// cells on a grid*grid board with `mines` mines, at 95% RTP. It panics if
// `revealed` exceeds the number of safe cells.
func MinesMultiplier(grid, mines, revealed int) float64 {
	safe := grid*grid - mines
	if revealed < 0 || revealed > safe {
		panic("domain: revealed out of range")
	}
	fair := 1.0
	for k := 0; k < revealed; k++ {
		fair *= float64(grid*grid-k) / float64(safe-k)
	}
	return fair * (1 - minesHouseEdge)
}
