-- Public DeltaGram card profile: the colour and NFT chosen by the owner, plus
-- the balances the native client shows on the profile.
CREATE TABLE IF NOT EXISTS deltament_card_prefs (
    user_id    BIGINT PRIMARY KEY,
    color_id   INT NOT NULL DEFAULT -1,
    nft_row_id BIGINT NOT NULL DEFAULT 0,
    nft_slug   TEXT NOT NULL DEFAULT '',
    nft_num    INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
