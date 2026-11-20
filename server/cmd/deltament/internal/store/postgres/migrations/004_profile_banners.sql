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
