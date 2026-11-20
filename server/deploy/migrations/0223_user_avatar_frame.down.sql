ALTER TABLE public.users
    DROP COLUMN IF EXISTS avatar_frame;

DROP INDEX IF EXISTS public.users_avatar_frame_idx;
