package domain

import "time"

// CardPrefs is the owner-chosen look of the DeltaGram wallet card.
type CardPrefs struct {
	UserID    int64
	ColorID   int
	NFTRowID  int64
	NFTSlug   string
	NFTNum    int
	NFTX       float64
	NFTY       float64
	NFTScale   float64
	NFTRotation float64
	UpdatedAt time.Time
}

// CardPublic is the row the public registry serves to every client: balances
// plus the card look, so another user's profile can render the same card.
type CardPublic struct {
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	TON        int64  `json:"ton_nanoton"`
	TONAddress string `json:"ton_address"`
	Stars    int64  `json:"stars"`
	ColorID  int    `json:"color_id"`
	NFTRowID int64  `json:"nft_row_id"`
	NFTSlug  string `json:"nft_slug"`
	NFTNum   int    `json:"nft_num"`
	NFTX       float64 `json:"nft_x"`
	NFTY       float64 `json:"nft_y"`
	NFTScale   float64 `json:"nft_scale"`
	NFTRotation float64 `json:"nft_rotation"`
}

// CardHistoryEntry is a real ledger movement on either the TON or Stars card.
type CardHistoryEntry struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	Amount       int64  `json:"amount"`
	Reason       string `json:"reason"`
	Title        string `json:"title"`
	Created      int64  `json:"created"`
	PeerID       int64  `json:"peer_id"`
	PeerName     string `json:"peer_name"`
	PeerUsername string `json:"peer_username"`
}
