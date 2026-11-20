-- A diamond preset for the "Мажор" verification mark.
--
-- The document is a real custom emoji from the Topics set (alt 💎), so clients
-- can resolve it through messages.getCustomEmojiDocuments. It is a separate
-- document id from the Premium preset on purpose: the catalogue is keyed by
-- document id, and both presets must be selectable independently.
INSERT INTO public.verification_icons (document_id, owner_bot_id, name, active, created_at, updated_at)
VALUES (5309958691854754293, 0, 'Мажор', true, now(), now())
ON CONFLICT (document_id) DO NOTHING;
