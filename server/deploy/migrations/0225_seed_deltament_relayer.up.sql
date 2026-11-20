-- Service account that escrows gifts listed on the Deltament marketplace.
--
-- A gift cannot be sold by the marketplace while it stays on the seller's
-- profile: the seller could hide/convert it mid-auction. When a user lists a
-- gift, ownership is moved here ("DeltaMent relayer"), and the buyer receives
-- it from this account on claim. The id is fixed so the marketplace and the
-- gift service agree without configuration.

INSERT INTO public.users (
    id, access_hash, phone, first_name, last_name, username, country_code,
    created_at, updated_at, verified, support, about, last_seen_at,
    default_history_ttl_period, is_bot, bot_info_version, premium_expires_at,
    emoji_status_document_id, emoji_status_until, color_set, color,
    color_background_emoji_id, profile_color_set, profile_color,
    profile_color_background_emoji_id
) VALUES (
    1780243401, 1780243401001, '', 'DeltaMent relayer', '', 'deltament_relayer', '',
    now(), now(), false, false,
    'Служебный аккаунт Deltament: хранит подарки до передачи покупателю.',
    0, 0, false, 0, NULL, 0, 0, false, 0, 0, false, 0, 0
)
ON CONFLICT (id) DO UPDATE SET
    access_hash = EXCLUDED.access_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    username = EXCLUDED.username,
    about = EXCLUDED.about,
    updated_at = now();
