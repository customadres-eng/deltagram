package domain

// Gates is a scatter-pays, tumbling slot in the style of a Greek-gods slot:
// a 6x5 grid, symbols pay by total count anywhere on the board (8 or more of a
// kind), winning symbols vanish and new ones tumble in, and golden multiplier
// orbs multiply the whole spin. RTP is pinned near 95% by the paytable and
// weights; gates_test.go asserts it by Monte-Carlo.

const (
	gatesRows = 5
	gatesCols = 6
	gatesCell = gatesRows * gatesCols // 30
	// gatesMaxTumbles caps one spin's cascade so a pathological seed cannot
	// tumble forever.
	gatesMaxTumbles = 8
	// gatesMinScatter is the count of a kind that starts paying.
	gatesMinScatter = 8
	// gatesOrb marks the multiplier-orb symbol index.
	gatesOrb = 8
)

// gatesWeight maps each symbol index to its reel weight (higher == common).
// Indexes 0..7 are scatter-pay symbols, 8 is the multiplier orb.
var gatesWeight = [9]uint64{
	0: 60, // low A
	1: 60, // low B
	2: 60, // low C
	3: 50, // low D
	4: 50, // low E
	5: 40, // mid crown
	6: 30, // mid hourglass
	7: 20, // high ring
	8: 6,  // multiplier orb
}

var gatesWeightTotal = func() uint64 {
	var t uint64
	for _, w := range gatesWeight {
		t += w
	}
	return t
}()

// gatesPay is the payout multiplier for a count of a kind, per symbol. Each
// symbol has three tiers: 8-9, 10-11 and 12+ of a kind. The multiplier is
// expressed as a fraction of the stake.
var gatesPay = [8][3]float64{
	{0.46, 1.12, 3.76}, // low A
	{0.46, 1.12, 3.76}, // low B
	{0.46, 1.12, 3.76}, // low C
	{0.56, 1.42, 4.67}, // low D
	{0.56, 1.42, 4.67}, // low E
	{0.91, 2.84, 9.44}, // crown
	{1.42, 3.76, 13.2}, // hourglass
	{2.34, 6.6, 24.4},  // ring
}

// gatesOrbValues are the possible multiplier-orb values with their weights.
var gatesOrbValues = []struct {
	value  int64
	weight uint64
}{
	{2, 100}, {3, 60}, {4, 40}, {5, 30}, {8, 20},
	{10, 12}, {15, 8}, {20, 6}, {25, 4}, {50, 2},
}

// GatesOrb is one multiplier orb that landed on the board: its board cell and
// the multiplier value it carries.
type GatesOrb struct {
	Cell  int   `json:"cell"`
	Value int64 `json:"value"`
}

// GatesTumble is one resolved cascade step.
type GatesTumble struct {
	// Removed are the cell indexes (col*rows+row) that won and were cleared.
	Removed []int `json:"removed"`
	// Orbs are the multiplier orbs that were present and consumed in this step.
	Orbs []GatesOrb `json:"orbs,omitempty"`
	// Win is this step's base scatter win, in stake multiples, before the orb
	// multiplier is applied.
	Win float64 `json:"win"`
	// Grid is the board after this tumble's refill, so the client can replay the
	// cascade without reproducing the RNG.
	Grid [gatesCell]int `json:"grid"`
}

// GatesResult is the outcome of one spin, ready to replay on the client.
type GatesResult struct {
	// Grid is the final 6x5 grid, col-major (index = col*5+row, row 0 bottom).
	Grid [gatesCell]int `json:"grid"`
	// Initial is the grid before the first tumble (for the opening render).
	Initial [gatesCell]int `json:"initial"`
	// Tumbles is the cascade, in order.
	Tumbles []GatesTumble `json:"tumbles"`
	// OrbTotal is the summed multiplier-orb value for the spin (1 when none).
	OrbTotal int64 `json:"orb_total"`
	// Multiplier is the total payout in stake multiples.
	Multiplier float64 `json:"multiplier"`
}

// GatesSpin resolves one spin with tumbling and multiplier orbs.
func (r *RNG) GatesSpin() GatesResult {
	var res GatesResult
	grid := r.drawGatesGrid()
	res.Initial = grid

	var base float64
	var orbTotal int64
	for step := 0; step < gatesMaxTumbles; step++ {
		counts := [8]int{}
		orbCells := []int{}
		var orbs []GatesOrb
		var orbSum int64
		for i := 0; i < gatesCell; i++ {
			if grid[i] == gatesOrb {
				orbCells = append(orbCells, i)
				v := r.drawOrbValue()
				orbs = append(orbs, GatesOrb{Cell: i, Value: v})
				orbSum += v
				continue
			}
			counts[grid[i]]++
		}
		removed := append([]int{}, orbCells...)
		var win float64
		for sym := 0; sym < 8; sym++ {
			c := counts[sym]
			if c < gatesMinScatter {
				continue
			}
			var mult float64
			switch {
			case c >= 12:
				mult = gatesPay[sym][2]
			case c >= 10:
				mult = gatesPay[sym][1]
			default:
				mult = gatesPay[sym][0]
			}
			win += mult
			for i := 0; i < gatesCell; i++ {
				if grid[i] == sym {
					removed = append(removed, i)
				}
			}
		}
		if win == 0 && len(orbCells) == 0 {
			break
		}
		// A multiplier orb multiplies the win of the tumble it lands in; orbs in
		// a no-win tumble are simply consumed and clear space for the next fall.
		if orbSum > 1 {
			win *= float64(orbSum)
		}
		orbTotal += orbSum
		base += win
		grid = r.tumbleGates(grid, removed)
		res.Tumbles = append(res.Tumbles, GatesTumble{Removed: removed, Orbs: orbs, Win: win, Grid: grid})
	}
	res.Grid = grid
	if orbTotal == 0 {
		orbTotal = 1
	}
	res.OrbTotal = orbTotal
	res.Multiplier = base
	return res
}

func (r *RNG) drawGatesGrid() [gatesCell]int {
	var g [gatesCell]int
	for i := 0; i < gatesCell; i++ {
		g[i] = r.drawGatesSymbol()
	}
	return g
}

func (r *RNG) drawGatesSymbol() int {
	roll := uint64(r.IntN(int(gatesWeightTotal)))
	for sym, w := range gatesWeight {
		if roll < w {
			return sym
		}
		roll -= w
	}
	return 0 // unreachable
}

func (r *RNG) drawOrbValue() int64 {
	var total uint64
	for _, o := range gatesOrbValues {
		total += o.weight
	}
	roll := uint64(r.IntN(int(total)))
	for _, o := range gatesOrbValues {
		if roll < o.weight {
			return o.value
		}
		roll -= o.weight
	}
	return gatesOrbValues[0].value
}

// tumbleGates clears the removed cells, lets the symbols above fall and draws
// fresh symbols at the top of each column.
func (r *RNG) tumbleGates(grid [gatesCell]int, removed []int) [gatesCell]int {
	cleared := make(map[int]bool, len(removed))
	for _, i := range removed {
		cleared[i] = true
	}
	for col := 0; col < gatesCols; col++ {
		// Collect the surviving symbols bottom-up, then fill the rest from the
		// top with new draws.
		var keep []int
		for row := 0; row < gatesRows; row++ {
			if !cleared[col*gatesRows+row] {
				keep = append(keep, grid[col*gatesRows+row])
			}
		}
		for row := 0; row < gatesRows; row++ {
			if row < len(keep) {
				grid[col*gatesRows+row] = keep[row]
			} else {
				grid[col*gatesRows+row] = r.drawGatesSymbol()
			}
		}
	}
	return grid
}
