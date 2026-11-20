-- Periodic online-presence samples for the admin console peak-online tile.
--
-- users.last_seen_at is overwritten on every activity, so the database cannot
-- answer "how many accounts were online at 14:05 yesterday". The live server
-- samples the same online predicate it already exposes as the five-minute count
-- and appends one row per sample; the console then takes max(online) over the
-- selected window.
CREATE TABLE IF NOT EXISTS presence_online_samples (
    id         BIGSERIAL PRIMARY KEY,
    sampled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    online     BIGINT NOT NULL CHECK (online >= 0)
);

-- Peak lookups are always "newest window first", so the timestamp index is the
-- only access path the tile needs.
CREATE INDEX IF NOT EXISTS presence_online_samples_time
    ON presence_online_samples (sampled_at DESC);
