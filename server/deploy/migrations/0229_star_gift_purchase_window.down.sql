ALTER TABLE public.star_gift_catalog_revisions
    DROP CONSTRAINT IF EXISTS star_gift_catalog_revision_purchase_window_check;

ALTER TABLE public.star_gift_catalog_revisions
    DROP COLUMN IF EXISTS purchase_window_timezone,
    DROP COLUMN IF EXISTS purchase_window_duration_minutes,
    DROP COLUMN IF EXISTS purchase_window_start_minute,
    DROP COLUMN IF EXISTS purchase_window_enabled;
