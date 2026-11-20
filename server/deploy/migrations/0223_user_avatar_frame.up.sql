-- DeltaGram avatar frames.
--
-- A frame is a purely cosmetic ring painted by the DeltaGram desktop client
-- around a user's avatar. The id stored here is one of the well-known frame
-- ids understood by the client (see internal/domain/avatar_frame.go); an empty
-- string means "no frame" and is the default for every account.
ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS avatar_frame text NOT NULL DEFAULT '';

-- The public frame registry only ever reads rows that actually carry a frame,
-- so a partial index keeps the registry query tiny even with millions of users.
CREATE INDEX IF NOT EXISTS users_avatar_frame_idx
    ON public.users (avatar_frame)
    WHERE avatar_frame <> '';
