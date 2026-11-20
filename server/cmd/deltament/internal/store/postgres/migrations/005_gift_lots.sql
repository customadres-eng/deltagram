-- Profile banners uploaded through Deltament. The binary lives here so the
-- web app needs no extra volume, and users.avatar_banner only carries the
-- version token clients use for cache busting.
CREATE TABLE IF NOT EXISTS deltament_profile_banners (
    user_id    bigint PRIMARY KEY,
    token      text NOT NULL,
    mime       text NOT NULL,
    bytes      bytea NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Escrowed marketplace gifts are moved between accounts without a real
-- message, but peer_star_gifts requires a positive unique msg_id for every
-- user-owned row. The sequence hands out high synthetic ids that cannot
-- collide with the low per-user message-box ids.
CREATE SEQUENCE IF NOT EXISTS deltament_gift_message_seq
    AS bigint START WITH 1500000000 INCREMENT BY 1;
