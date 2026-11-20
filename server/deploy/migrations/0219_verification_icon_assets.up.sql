-- Real badge artwork and multiple third-party verifications per profile.
--
-- 0217/0218 seeded Unicode custom emoji as placeholders: the badge slot draws a
-- custom emoji document, so those presets rendered as an emoji glyph rather than
-- as the operator's artwork. The documents inserted here are static WebP custom
-- emoji bundled into the server binary (internal/seed/verificationicons) and
-- seeded at startup, so the client draws the supplied images.
--
-- The unique key changes from "one mark per peer" to "one mark per peer, per
-- verifier, per icon": a profile may now carry several third-party
-- verifications, while replaying the same grant still updates in place.

-- The organization name is stored separately from the sentence so the projection
-- can render the sentence in the reader's language. description remains as the
-- stored fallback for marks created before this migration.
ALTER TABLE public.custom_verifications
    ADD COLUMN IF NOT EXISTS organization_name text NOT NULL DEFAULT ''
        CHECK (octet_length(organization_name) <= 512);

-- The previous panel always stored the sentence in Russian:
--   Аккаунт верифицирован организацией «<name>».
-- Lift the name out of it for every existing mark, so those marks localize too
-- instead of freezing in the language they were granted in.
UPDATE public.custom_verifications
SET organization_name = substring(description from 'организацией «([^»]+)»'),
    updated_at = now(),
    version = version + 1
WHERE organization_name = ''
  AND description ~ 'организацией «[^»]+»';

-- Move existing marks onto the new artwork. The mapping is by organization
-- because the old preset chose a placeholder emoji per organization, not per
-- artwork: the Major mark may point at any of the retired documents.
UPDATE public.custom_verifications
SET icon_document_id = CASE organization_name
        WHEN 'Major'     THEN 5310000000000000101
        WHEN 'Business'  THEN 5310000000000000102
        WHEN 'Diamond'   THEN 5310000000000000103
        WHEN 'Crown'     THEN 5310000000000000104
        WHEN 'Official'  THEN 5310000000000000104
        WHEN 'Star'      THEN 5310000000000000105
        WHEN 'Partner'   THEN 5310000000000000106
        WHEN 'DeltaGram' THEN 5310000000000000107
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE organization_name IN (
    'Major', 'Business', 'Diamond', 'Crown', 'Official', 'Star', 'Partner',
    'DeltaGram'
);

-- Fallback for rows whose sentence could not be parsed: the two ids production
-- actually used before 0219.
UPDATE public.custom_verifications
SET icon_document_id = 5310000000000000104, updated_at = now(), version = version + 1
WHERE icon_document_id = 5467406098367521267;

UPDATE public.custom_verifications
SET icon_document_id = 5310000000000000102, updated_at = now(), version = version + 1
WHERE icon_document_id = 5359785904535774578;

-- The verifier's own settings icon follows the company it is configured under.
UPDATE public.bot_verifier_settings
SET icon_document_id = CASE company_name
        WHEN 'Major'     THEN 5310000000000000101
        WHEN 'Business'  THEN 5310000000000000102
        WHEN 'Diamond'   THEN 5310000000000000103
        WHEN 'Crown'     THEN 5310000000000000104
        WHEN 'Official'  THEN 5310000000000000104
        WHEN 'Star'      THEN 5310000000000000105
        WHEN 'Partner'   THEN 5310000000000000106
        WHEN 'DeltaGram' THEN 5310000000000000107
        ELSE icon_document_id
    END,
    updated_at = now(),
    version = version + 1
WHERE company_name IN (
    'Major', 'Business', 'Diamond', 'Crown', 'Official', 'Star', 'Partner',
    'DeltaGram'
);

-- Collapse any rows the new identity would make duplicates of, keeping the
-- newest. This is a no-op on a database that never had two marks on one peer.
DELETE FROM public.custom_verifications cv
USING public.custom_verifications newer
WHERE cv.peer_type = newer.peer_type
  AND cv.peer_id = newer.peer_id
  AND cv.verifier_bot_id = newer.verifier_bot_id
  AND cv.icon_document_id = newer.icon_document_id
  AND cv.id < newer.id;

ALTER TABLE public.custom_verifications
    DROP CONSTRAINT IF EXISTS custom_verifications_peer_once;

ALTER TABLE public.custom_verifications
    ADD CONSTRAINT custom_verifications_peer_icon_once
        UNIQUE (peer_type, peer_id, verifier_bot_id, icon_document_id);

-- Retire the placeholder emoji presets. Marks already granted keep their icon id
-- denormalised; the ids used in production were remapped above.
DELETE FROM public.verification_icons
WHERE document_id IN (
    5359785904535774578,
    5427009714745517609,
    5471952986970267163,
    5467406098367521267,
    5357080225463149588,
    5458799228719472718,
    5420315771991497307,
    5409008750893734809,
    5422683699130933153,
    5309958691854754293
);

INSERT INTO public.verification_icons (document_id, owner_bot_id, name, active, created_at, updated_at)
VALUES
    (5310000000000000101, 0, 'Major',     true, now(), now()),
    (5310000000000000102, 0, 'Business',  true, now(), now()),
    (5310000000000000103, 0, 'Diamond',   true, now(), now()),
    (5310000000000000104, 0, 'Crown',     true, now(), now()),
    (5310000000000000105, 0, 'Star',      true, now(), now()),
    (5310000000000000106, 0, 'Partner',   true, now(), now()),
    (5310000000000000107, 0, 'DeltaGram', true, now(), now())
ON CONFLICT (document_id) DO UPDATE
SET name = EXCLUDED.name,
    active = true,
    updated_at = now();
