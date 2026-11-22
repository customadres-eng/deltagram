-- Old static frame assignments must not linger after the catalog is replaced.
UPDATE public.users SET avatar_frame = '', updated_at = now()
WHERE avatar_frame <> ''
  AND avatar_frame NOT IN ('blue', 'fire', 'green', 'holographic_glass_ring', 'liquid_chrome_ring');
