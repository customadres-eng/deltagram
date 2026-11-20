-- Revert the real-artwork catalogue and the multi-mark identity.
--
-- This is a structural rollback, not a data restore: the placeholder emoji
-- documents 0219 removed are not re-seeded (their bodies lived only in the old
-- presets), and marks are left pointing at the WebP documents. What is restored
-- is the schema the previous binary expects: one mark per peer, no
-- organization_name column, and none of the 0219 catalogue rows.

-- Collapse a profile back to its newest mark before the peer-wide unique
-- constraint can exist again. Older marks lose; the newest one is what the
-- previous single-slot projection would have shown anyway.
DELETE FROM public.custom_verifications cv
USING public.custom_verifications newer
WHERE cv.peer_type = newer.peer_type
  AND cv.peer_id = newer.peer_id
  AND cv.id < newer.id;

ALTER TABLE public.custom_verifications
    DROP CONSTRAINT IF EXISTS custom_verifications_peer_icon_once;

ALTER TABLE public.custom_verifications
    ADD CONSTRAINT custom_verifications_peer_once
        UNIQUE (peer_type, peer_id);

ALTER TABLE public.custom_verifications
    DROP COLUMN IF EXISTS organization_name;

-- Only the rows this migration seeded, and only while they still carry the
-- seeded label. A mark granted with one of these icons keeps its denormalised
-- icon id and is untouched.
DELETE FROM public.verification_icons
WHERE owner_bot_id = 0
  AND document_id BETWEEN 5310000000000000101 AND 5310000000000000107
  AND name IN ('Major', 'Business', 'Diamond', 'Crown', 'Star', 'Partner', 'DeltaGram');
