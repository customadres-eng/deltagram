-- Casino Mini App bot (@deltagram_casino_bot).
--
-- The casino is a Telegram Mini App: the bot's token (<id>:<secret>) is what the
-- DeltaGram client uses to sign the WebApp init_data, and what the casino
-- service uses to verify it. The token secret is fixed here so the casino's
-- CASINO_BOT_TOKEN can be configured once and never drift.

INSERT INTO public.users (
    id, access_hash, phone, first_name, last_name, username, country_code,
    created_at, updated_at, verified, support, about, last_seen_at,
    default_history_ttl_period, is_bot, bot_info_version, premium_expires_at,
    emoji_status_document_id, emoji_status_until, color_set, color,
    color_background_emoji_id, profile_color_set, profile_color,
    profile_color_background_emoji_id
) VALUES (
    1780243404, 1780243404001, '', 'DeltaGram Casino', '', 'deltagram_casino_bot', '',
    now(), now(), false, false,
    'Служебный бот казино DeltaGram: краш, мины, апгрейд и слоты.',
    0, 0, true, 1, NULL, 0, 0, false, 0, 0, false, 0, 0
)
ON CONFLICT (id) DO UPDATE SET
    access_hash = EXCLUDED.access_hash,
    phone = EXCLUDED.phone,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    username = EXCLUDED.username,
    verified = EXCLUDED.verified,
    support = EXCLUDED.support,
    about = EXCLUDED.about,
    is_bot = EXCLUDED.is_bot,
    bot_info_version = GREATEST(public.users.bot_info_version, EXCLUDED.bot_info_version),
    updated_at = now();

INSERT INTO public.bots (
    bot_user_id, owner_user_id, token_secret, description, commands,
    bot_chat_history, bot_nochats, inline_placeholder, created_at, updated_at,
    menu_button_type, menu_button_text, menu_button_url, bot_inline_geo
) VALUES (
    1780243404, 1780243200, 'REPLACE_WITH_CASINO_BOT_TOKEN',
    'Казино DeltaGram: краш, мины, апгрейд, слоты.',
    '[
        {"command": "start", "description": "открыть казино"},
        {"command": "balance", "description": "баланс казино"},
        {"command": "help", "description": "справка"}
    ]'::jsonb,
    false, true, '', now(), now(), 0, '', '', false
)
ON CONFLICT (bot_user_id) DO UPDATE SET
    owner_user_id = EXCLUDED.owner_user_id,
    token_secret = EXCLUDED.token_secret,
    description = EXCLUDED.description,
    commands = EXCLUDED.commands,
    bot_chat_history = EXCLUDED.bot_chat_history,
    bot_nochats = EXCLUDED.bot_nochats,
    inline_placeholder = EXCLUDED.inline_placeholder,
    updated_at = now();

INSERT INTO public.peer_usernames (
    username_lower, username, peer_type, peer_id, active, editable, sort_order, updated_at
)
VALUES ('deltagram_casino_bot', 'deltagram_casino_bot', 'user', 1780243404, true, true, 0, now())
ON CONFLICT (username_lower) DO UPDATE SET
    username = EXCLUDED.username,
    peer_type = EXCLUDED.peer_type,
    peer_id = EXCLUDED.peer_id,
    active = EXCLUDED.active,
    editable = EXCLUDED.editable,
    updated_at = now();

INSERT INTO public.read_model_versions (model, owner_user_id, peer_type, peer_id, version, updated_at, hash)
VALUES
    ('contact_account', 1780243404, 'user', 1780243404, 1, now(), 1780243404001),
    ('channel_active_memberships', 1780243404, 'user', 1780243404, 1, now(), 1780243404002)
ON CONFLICT (model, owner_user_id, peer_type, peer_id) DO UPDATE SET
    version = GREATEST(public.read_model_versions.version, EXCLUDED.version),
    updated_at = now(),
    hash = EXCLUDED.hash;
