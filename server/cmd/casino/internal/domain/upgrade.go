package domain

// Upgrade is the single-shot "all or nothing" upgrade (the upgrader.pro model):
// the player risks a stake and picks a target multiplier, and either wins the
// upgraded amount or loses everything. The win chance is inversely proportional
// to the target, so chance * multiplier = RTP is constant.
//
// RTP 95%: chance = (1 - edge) / multiplier. Expected return is
// chance * multiplier = 1 - edge = 95% for any target.

const upgradeHouseEdge = 0.05

// UpgradeChance returns the win probability for a target payout multiplier at
// 95% RTP. A multiplier at or below the RTP is a guaranteed (but shrinking)
// win, so the result is capped at 1.
func UpgradeChance(multiplier float64) float64 {
	if multiplier <= 0 {
		return 0
	}
	c := (1 - upgradeHouseEdge) / multiplier
	if c > 1 {
		return 1
	}
	return c
}

// UpgradeMultiplier returns the payout multiplier that corresponds to a chosen
// win chance at 95% RTP. A chance at or above 1-edge is a guaranteed win, so
// the multiplier is floored at 1-edge (the player cannot force a positive
// expectation by demanding a very high chance).
func UpgradeMultiplier(chance float64) float64 {
	if chance <= 0 {
		return 0
	}
	if chance >= 1-upgradeHouseEdge {
		return 1 - upgradeHouseEdge
	}
	return (1 - upgradeHouseEdge) / chance
}

// UpgradeRoll resolves one upgrade attempt: it returns true when the player
// wins (u < chance). `u` is a uniform draw in [0,1), so the roll is
// deterministic given the RNG state.
func UpgradeRoll(u, chance float64) bool {
	if chance <= 0 {
		return false
	}
	if chance >= 1 {
		return true
	}
	return u < chance
}

// Upgrade draws and resolves the next upgrade attempt for the given win chance.
func (r *RNG) Upgrade(chance float64) bool { return UpgradeRoll(r.Uniform(), chance) }
