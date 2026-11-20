-- Roll back the second icon set to the 0219 placeholder ids.

UPDATE public.custom_verifications
SET icon_document_id = CASE icon_document_id
        WHEN 5310000000000000111 THEN 5310000000000000101
        WHEN 5310000000000000112 THEN 5310000000000000102
        WHEN 5310000000000000113 THEN 5310000000000000103
        WHEN 5310000000000000114 THEN 5310000000000000104
        WHEN 5310000000000000115 THEN 5310000000000000105
        WHEN 5310000000000000116 THEN 5310000000000000106
        WHEN 5310000000000000117 THEN 5310000000000000107
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE icon_document_id BETWEEN 5310000000000000111 AND 5310000000000000117;

DELETE FROM public.custom_verifications cv
USING public.custom_verifications newer
WHERE cv.peer_type = newer.peer_type
  AND cv.peer_id = newer.peer_id
  AND cv.verifier_bot_id = newer.verifier_bot_id
  AND cv.icon_document_id = newer.icon_document_id
  AND cv.id < newer.id;

UPDATE public.bot_verifier_settings
SET icon_document_id = CASE icon_document_id
        WHEN 5310000000000000111 THEN 5310000000000000101
        WHEN 5310000000000000112 THEN 5310000000000000102
        WHEN 5310000000000000113 THEN 5310000000000000103
        WHEN 5310000000000000114 THEN 5310000000000000104
        WHEN 5310000000000000115 THEN 5310000000000000105
        WHEN 5310000000000000116 THEN 5310000000000000106
        WHEN 5310000000000000117 THEN 5310000000000000107
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE icon_document_id BETWEEN 5310000000000000111 AND 5310000000000000117;

DELETE FROM public.verification_icons
WHERE owner_bot_id = 0
  AND document_id BETWEEN 5310000000000000111 AND 5310000000000000117
  AND name IN ('Major', 'Business', 'Diamond', 'Crown', 'Star', 'Partner', 'DeltaGram');
