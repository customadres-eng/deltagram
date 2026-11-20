-- Preset icons for direct third-party verification grants.
--
-- These are the custom emoji documents already shipped with the emoji data set.
-- The catalogue is what makes an icon grantable, so seeding the presets here is
-- what lets the admin panel offer Business / DeltaGram / Premium / ... out of the
-- box. ON CONFLICT keeps this non-destructive: an operator who has already added
-- or renamed one of these document ids keeps their row untouched.
INSERT INTO public.verification_icons (document_id, owner_bot_id, name, active, created_at, updated_at)
VALUES
    (5359785904535774578, 0, 'Business', true, now(), now()),
    (5427009714745517609, 0, 'DeltaGram', true, now(), now()),
    (5471952986970267163, 0, 'Premium', true, now(), now()),
    (5467406098367521267, 0, 'Official', true, now(), now()),
    (5357080225463149588, 0, 'Partner', true, now(), now()),
    (5458799228719472718, 0, 'Star', true, now(), now()),
    (5420315771991497307, 0, 'Fire', true, now(), now()),
    (5409008750893734809, 0, 'Trophy', true, now(), now()),
    (5422683699130933153, 0, 'ID Card', true, now(), now())
ON CONFLICT (document_id) DO NOTHING;
