-- Operator-issued usernames may contain Cyrillic letters. PostgreSQL's lower()
-- follows the database collation, and a C/POSIX locale does not fold non-ASCII
-- letters: a row with username 'Привет' and Go-computed username_lower 'привет'
-- would then violate the case-normalisation CHECKs even though the application
-- normalised it correctly. Pin the comparisons to the built-in ICU root
-- collation so Unicode case folding is locale-independent, and keep the users
-- uniqueness index in step with the same folding rule.
ALTER TABLE public.collectible_usernames
    DROP CONSTRAINT IF EXISTS collectible_usernames_username_lower_check;

ALTER TABLE public.collectible_usernames
    ADD CONSTRAINT collectible_usernames_username_lower_check
    CHECK (username_lower <> '' AND lower(username COLLATE "und-x-icu") = username_lower);

ALTER TABLE public.peer_usernames
    DROP CONSTRAINT IF EXISTS peer_usernames_username_case_check;

ALTER TABLE public.peer_usernames
    ADD CONSTRAINT peer_usernames_username_case_check
    CHECK (lower(username COLLATE "und-x-icu") = username_lower);

DROP INDEX IF EXISTS public.users_username_lower_unique_idx;

CREATE UNIQUE INDEX users_username_lower_unique_idx
    ON public.users USING btree (lower((username)::text COLLATE "und-x-icu"))
    WHERE ((username)::text <> '');
