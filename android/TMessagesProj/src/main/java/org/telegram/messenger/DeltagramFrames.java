package org.telegram.messenger;

import android.graphics.Canvas;
import android.graphics.Rect;
import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;
import android.view.View;

import org.json.JSONObject;
import org.telegram.ui.Components.RLottieDrawable;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.File;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import java.util.WeakHashMap;

/** Five animated avatar rings supplied as Lottie JSON, assigned via the public registry. */
public final class DeltagramFrames {
    public static final String REGISTRY_URL = "https://dltagram.duckdns.org/api/frames/registry";
    private static final String[] KINDS = {"blue", "fire", "green", "holographic_glass_ring", "liquid_chrome_ring"};
    private static final Handler handler = new Handler(Looper.getMainLooper());
    private static final Object lock = new Object();
    private static final Map<Long, String> frames = new HashMap<>();
    private static final Map<String, RLottieDrawable> animations = new HashMap<>();
    private static final WeakHashMap<View, Boolean> visibleViews = new WeakHashMap<>();
    private static volatile boolean started;
    private static volatile boolean inFlight;

    private DeltagramFrames() {}

    public static void ensureStarted() {
        if (started) return;
        synchronized (lock) {
            if (started) return;
            started = true;
        }
        handler.post(DeltagramFrames::refresh);
    }

    public static String frameFor(long id) {
        if (id == 0) return null;
        ensureStarted();
        synchronized (lock) {
            return frames.get(id);
        }
    }

    private static boolean supported(String kind) {
        for (String item : KINDS) if (item.equals(kind)) return true;
        return false;
    }

    private static void refresh() {
        if (inFlight) return;
        inFlight = true;
        new Thread(() -> {
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL(REGISTRY_URL).openConnection();
                connection.setConnectTimeout(10000);
                connection.setReadTimeout(10000);
                connection.setRequestProperty("User-Agent", "DeltaGramAndroid");
                if (connection.getResponseCode() != HttpURLConnection.HTTP_OK) return;
                StringBuilder body = new StringBuilder();
                try (BufferedReader reader = new BufferedReader(new InputStreamReader(connection.getInputStream(), StandardCharsets.UTF_8))) {
                    String line;
                    while ((line = reader.readLine()) != null) body.append(line);
                }
                JSONObject list = new JSONObject(body.toString()).optJSONObject("frames");
                Map<Long, String> next = new HashMap<>();
                if (list != null) {
                    java.util.Iterator<String> keys = list.keys();
                    while (keys.hasNext()) {
                        String key = keys.next();
                        String kind = list.optString(key, "");
                        if (!supported(kind)) continue;
                        try {
                            next.put(Long.parseLong(key), kind);
                        } catch (NumberFormatException ignored) {}
                    }
                }
                synchronized (lock) {
                    frames.clear();
                    frames.putAll(next);
                }
                handler.post(() -> {
                    NotificationCenter.getGlobalInstance().postNotificationName(NotificationCenter.dialogsNeedReload);
                    invalidateViews();
                });
            } catch (Exception ignored) {
                // Keep the latest successful registry while offline.
            } finally {
                if (connection != null) connection.disconnect();
                inFlight = false;
                handler.postDelayed(DeltagramFrames::refresh, 60000);
            }
        }, "DeltaGramFrames").start();
    }

    private static void invalidateViews() {
        boolean anyVisible = false;
        for (View view : visibleViews.keySet()) {
            if (view.isAttachedToWindow() && view.getVisibility() == View.VISIBLE) {
                view.invalidate();
                anyVisible = true;
            }
        }
        // Avoid decoding Lottie frames indefinitely after leaving the profile
        // or scrolling every framed avatar off screen.
        if (!anyVisible) {
            for (RLottieDrawable drawable : animations.values()) drawable.stop();
        }
    }

    private static RLottieDrawable animation(String kind) {
        RLottieDrawable drawable = animations.get(kind);
        if (drawable != null) return drawable;
        if (!supported(kind)) return null;
        try (InputStream stream = ApplicationLoader.applicationContext.getAssets().open("avatar_frames/" + kind + ".json")) {
            java.io.ByteArrayOutputStream buffer = new java.io.ByteArrayOutputStream();
            byte[] bytes = new byte[8192];
            int read;
            while ((read = stream.read(bytes)) != -1) buffer.write(bytes, 0, read);
            String json = buffer.toString("UTF-8");
            // The native Lottie renderer accepts JSON directly; no TGS decompression required.
            drawable = new RLottieDrawable(new File(ApplicationLoader.applicationContext.getCacheDir(), kind + ".json"),
                    json, 192, 192, null, true, null, 0, false);
            drawable.setCallback(new android.graphics.drawable.Drawable.Callback() {
                @Override public void invalidateDrawable(android.graphics.drawable.Drawable who) { invalidateViews(); }
                @Override public void scheduleDrawable(android.graphics.drawable.Drawable who, Runnable what, long when) {
                    handler.postAtTime(what, when);
                }
                @Override public void unscheduleDrawable(android.graphics.drawable.Drawable who, Runnable what) {
                    handler.removeCallbacks(what);
                }
            });
            animations.put(kind, drawable);
            drawable.start();
            return drawable;
        } catch (Exception ignored) {
            return null;
        }
    }

    public static void paint(Canvas canvas, Rect bounds, long userId) {
        paint(canvas, bounds, userId, null);
    }

    public static void paint(Canvas canvas, Rect bounds, long userId, View host) {
        paint(canvas, bounds, frameFor(userId), host);
    }

    public static void paint(Canvas canvas, Rect bounds, String kind) {
        paint(canvas, bounds, kind, null);
    }

    private static void paint(Canvas canvas, Rect bounds, String kind, View host) {
        if (canvas == null || bounds == null || bounds.width() < 12 || TextUtils.isEmpty(kind)) return;
        if (host != null) visibleViews.put(host, true);
        RLottieDrawable drawable = animation(kind);
        if (drawable == null) return;
        if (host != null && host.isAttachedToWindow() && !drawable.isRunning()) drawable.start();
        drawable.setBounds(bounds);
        drawable.draw(canvas);
    }
}
