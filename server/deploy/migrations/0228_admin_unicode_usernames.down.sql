-- Restore the locale-dependent case checks. This only succeeds while every
-- stored username folds identically under the database collation, so the down
-- migration is meant for a database that never received a Cyrillic name.
ALTER TABLE public.collectible_usernames
    DROP CONSTRAINT IF EXISTS collectible_usernames_username_lower_check;

ALTER TABLE public.collectible_usernames
    ADD CONSTRAINT collectible_usernames_username_lower_check
    CHECK (username_lower <> '' AND lower(username) = username_lower);

ALTER TABLE public.peer_usernames
    DROP CONSTRAINT IF EXISTS peer_usernames_username_case_check;

ALTER TABLE public.peer_usernames
    ADD CONSTRAINT peer_usernames_username_case_check
    CHECK (lower(username) = username_lower);

DROP INDEX IF EXISTS public.users_username_lower_unique_idx;

CREATE UNIQUE INDEX users_username_lower_unique_idx
    ON public.users USING btree (lower((username)::text))
    WHERE ((username)::text <> '');
