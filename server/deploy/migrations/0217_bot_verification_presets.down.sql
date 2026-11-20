-- Remove only the rows this migration seeded, and only while they still carry the
-- seeded label. A mark granted with one of these icons is untouched: the icon id
-- is denormalised onto the mark.
DELETE FROM public.verification_icons
WHERE owner_bot_id = 0
  AND (document_id, name) IN (
    (5359785904535774578, 'Business'),
    (5427009714745517609, 'DeltaGram'),
    (5471952986970267163, 'Premium'),
    (5467406098367521267, 'Official'),
    (5357080225463149588, 'Partner'),
    (5458799228719472718, 'Star'),
    (5420315771991497307, 'Fire'),
    (5409008750893734809, 'Trophy'),
    (5422683699130933153, 'ID Card')
  );
