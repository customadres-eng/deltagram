-- DeltaGram profile banners.
--
-- avatar_banner stores an opaque version token (not a URL) that is non-empty
-- only when the user has uploaded a banner through Deltament. Clients learn
-- about banners through the public registry and fetch the image itself from
-- Deltament, so the messenger never has to carry binary profile art.
ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS avatar_banner text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS users_avatar_banner_idx
    ON public.users (avatar_banner)
    WHERE avatar_banner <> '';
