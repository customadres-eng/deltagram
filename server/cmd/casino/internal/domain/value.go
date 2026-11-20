package domain

// NFT valuation. Market/catalog prices understate a collectible's real value by
// roughly 5x, so the base collection price is scaled up, and rarer attributes
// multiply it further. The black backdrop is the single most expensive trait,
// sitting above every rarity tier.

// marketPriceFactor scales the catalog (market) star price to the real value
// the casino prices an NFT at.
const marketPriceFactor = 5

// rarityMultiplier maps a trait's rarity permille (lower == rarer) to a value
// multiplier. It mirrors the tiers agreed with the operator: common through
// legendary.
func rarityMultiplier(permille int) int64 {
	switch {
	case permille <= 7:
		return 16 // legendary
	case permille <= 10:
		return 8 // epic
	case permille <= 14:
		return 4 // rare
	case permille <= 18:
		return 2 // uncommon
	default:
		return 1 // common
	}
}

// NftValue estimates the star value of an NFT from its base collection price,
// model rarity and backdrop rarity. isBlackBackdrop marks the top-tier black
// background, which out-ranks even a legendary attribute.
func NftValue(baseStars int64, modelPermille, backdropPermille int, isBlackBackdrop bool) int64 {
	value := baseStars * marketPriceFactor
	value *= rarityMultiplier(modelPermille)
	if isBlackBackdrop {
		value *= 32
	} else {
		value *= rarityMultiplier(backdropPermille)
	}
	return value
}
