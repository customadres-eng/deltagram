DROP INDEX IF EXISTS public.users_avatar_banner_idx;
ALTER TABLE public.users
    DROP COLUMN IF EXISTS avatar_banner;
