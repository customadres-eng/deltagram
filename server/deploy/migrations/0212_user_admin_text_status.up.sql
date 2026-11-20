CREATE TABLE public.user_admin_text_status (
    user_id bigint PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_admin_text_status_length CHECK (char_length(status) <= 64)
);
