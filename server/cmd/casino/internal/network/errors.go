package network

import "errors"

// Sentinel errors returned by the star gateway. The wallet maps them onto the
// casino's own error vocabulary at the HTTP boundary.
var (
	// ErrInsufficient means the player does not have enough stars on the main
	// server to cover a deposit.
	ErrInsufficient = errors.New("insufficient stars")

	// ErrStarsDisabled means the Admin API token is not configured, so star
	// deposit/withdraw is unavailable in this deployment.
	ErrStarsDisabled = errors.New("star movements are disabled")
)
