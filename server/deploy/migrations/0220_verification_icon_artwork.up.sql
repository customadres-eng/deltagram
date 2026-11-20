-- Real operator badge artwork (second pass).
--
-- 0219 shipped locally drawn placeholders under document ids ...101-...107.
-- The operator supplied the final artwork; the bundled WebP bodies are replaced
-- in internal/seed/verificationicons and re-imported under fresh document ids so
-- clients (and the idempotent startup seed, which never rewrites an existing
-- blob) actually pick up the new images. Every mark that pointed at a
-- placeholder id is remapped to its replacement, preserving existing badges.

-- Move marks first; the old and new ids can coexist only briefly, so collapse
-- the (peer, verifier, icon) identity before swapping.
UPDATE public.custom_verifications
SET icon_document_id = CASE icon_document_id
        WHEN 5310000000000000101 THEN 5310000000000000111
        WHEN 5310000000000000102 THEN 5310000000000000112
        WHEN 5310000000000000103 THEN 5310000000000000113
        WHEN 5310000000000000104 THEN 5310000000000000114
        WHEN 5310000000000000105 THEN 5310000000000000115
        WHEN 5310000000000000106 THEN 5310000000000000116
        WHEN 5310000000000000107 THEN 5310000000000000117
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE icon_document_id BETWEEN 5310000000000000101 AND 5310000000000000107;

DELETE FROM public.custom_verifications cv
USING public.custom_verifications newer
WHERE cv.peer_type = newer.peer_type
  AND cv.peer_id = newer.peer_id
  AND cv.verifier_bot_id = newer.verifier_bot_id
  AND cv.icon_document_id = newer.icon_document_id
  AND cv.id < newer.id;

UPDATE public.bot_verifier_settings
SET icon_document_id = CASE icon_document_id
        WHEN 5310000000000000101 THEN 5310000000000000111
        WHEN 5310000000000000102 THEN 5310000000000000112
        WHEN 5310000000000000103 THEN 5310000000000000113
        WHEN 5310000000000000104 THEN 5310000000000000114
        WHEN 5310000000000000105 THEN 5310000000000000115
        WHEN 5310000000000000106 THEN 5310000000000000116
        WHEN 5310000000000000107 THEN 5310000000000000117
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE icon_document_id BETWEEN 5310000000000000101 AND 5310000000000000107;

INSERT INTO public.verification_icons (document_id, owner_bot_id, name, active, created_at, updated_at)
VALUES
    (5310000000000000111, 0, 'Major',     true, now(), now()),
    (5310000000000000112, 0, 'Business',  true, now(), now()),
    (5310000000000000113, 0, 'Diamond',   true, now(), now()),
    (5310000000000000114, 0, 'Crown',     true, now(), now()),
    (5310000000000000115, 0, 'Star',      true, now(), now()),
    (5310000000000000116, 0, 'Partner',   true, now(), now()),
    (5310000000000000117, 0, 'DeltaGram', true, now(), now())
ON CONFLICT (document_id) DO UPDATE
SET name = EXCLUDED.name,
    active = true,
    updated_at = now();

-- Retire the placeholder catalogue entries. Marks no longer reference them
-- after the remap above.
DELETE FROM public.verification_icons
WHERE owner_bot_id = 0
  AND document_id BETWEEN 5310000000000000101 AND 5310000000000000107
  AND name IN ('Major', 'Business', 'Diamond', 'Crown', 'Star', 'Partner', 'DeltaGram');
