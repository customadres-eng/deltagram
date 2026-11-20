-- Remove only the row this migration seeded, and only while it still carries the
-- seeded label. A mark granted with this icon is untouched: the icon id is
-- denormalised onto the mark.
DELETE FROM public.verification_icons
WHERE owner_bot_id = 0
  AND document_id = 5309958691854754293
  AND name = 'Мажор';
