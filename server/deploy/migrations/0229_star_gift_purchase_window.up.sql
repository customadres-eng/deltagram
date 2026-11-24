-- Recurring daily purchase window for limited gift drops. LockedUntilDate can
-- only express a single scheduled release; a drop that may be bought "only at
-- 19:40 Kyiv time" needs a daily window instead. The window is evaluated in an
-- IANA timezone so Europe/Kyiv follows DST automatically.
ALTER TABLE public.star_gift_catalog_revisions
    ADD COLUMN purchase_window_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN purchase_window_start_minute smallint NOT NULL DEFAULT 0,
    ADD COLUMN purchase_window_duration_minutes smallint NOT NULL DEFAULT 0,
    ADD COLUMN purchase_window_timezone text NOT NULL DEFAULT '';

ALTER TABLE public.star_gift_catalog_revisions
    ADD CONSTRAINT star_gift_catalog_revision_purchase_window_check CHECK (
        (NOT purchase_window_enabled
            AND purchase_window_start_minute = 0
            AND purchase_window_duration_minutes = 0
            AND purchase_window_timezone = '')
        OR
        (purchase_window_enabled
            AND purchase_window_start_minute BETWEEN 0 AND 1439
            AND purchase_window_duration_minutes BETWEEN 1 AND 1440
            AND purchase_window_timezone <> '')
    );
