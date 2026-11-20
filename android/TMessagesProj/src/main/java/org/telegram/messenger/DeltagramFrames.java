/*
 * DeltaGram cosmetic avatar frames.
 *
 * The admin panel grants a frame id to a DeltaGram account; the public
 * registry below is polled anonymously so every client can paint the same
 * ring around that account's avatar without any MTProto payload changes.
 */

package org.telegram.messenger;

import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.LinearGradient;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.Shader;
import android.graphics.Typeface;
import android.graphics.SweepGradient;
import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

public final class DeltagramFrames {

    public static final String REGISTRY_URL = "https://dltagram.duckdns.org/api/frames/registry";

    private static final long REFRESH_INTERVAL = 60 * 1000L;
    private static final String USER_AGENT = "DeltaGramAndroid";

    private static final Object lock = new Object();
    private static final Map<Long, String> frames = new HashMap<>();
    private static final Handler handler = new Handler(Looper.getMainLooper());

    private static volatile boolean started;
    private static volatile boolean inFlight;

    private static final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private static final Path path = new Path();
    private static final RectF rect = new RectF();

    private DeltagramFrames() {
    }

    public static void ensureStarted() {
        if (started) {
            return;
        }
        synchronized (lock) {
            if (started) {
                return;
            }
            started = true;
        }
        handler.post(DeltagramFrames::refresh);
    }

    public static String frameFor(long userId) {
        if (userId == 0) {
            return null;
        }
        ensureStarted();
        synchronized (lock) {
            return frames.get(userId);
        }
    }

    private static void refresh() {
        if (inFlight) {
            return;
        }
        inFlight = true;
        new Thread(() -> {
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL(REGISTRY_URL).openConnection();
                connection.setConnectTimeout(10000);
                connection.setReadTimeout(10000);
                connection.setRequestProperty("User-Agent", USER_AGENT);
                connection.setRequestProperty("Accept", "application/json");
                if (connection.getResponseCode() != HttpURLConnection.HTTP_OK) {
                    return;
                }
                StringBuilder body = new StringBuilder();
                try (BufferedReader reader = new BufferedReader(new InputStreamReader(connection.getInputStream(), StandardCharsets.UTF_8))) {
                    String line;
                    while ((line = reader.readLine()) != null) {
                        body.append(line);
                    }
                }
                JSONObject root = new JSONObject(body.toString());
                JSONObject list = root.optJSONObject("frames");
                Map<Long, String> next = new HashMap<>();
                if (list != null) {
                    java.util.Iterator<String> keys = list.keys();
                    while (keys.hasNext()) {
                        String key = keys.next();
                        String kind = list.optString(key, "");
                        if (TextUtils.isEmpty(kind)) {
                            continue;
                        }
                        try {
                            next.put(Long.parseLong(key), kind);
                        } catch (NumberFormatException ignored) {
                        }
                    }
                }
                synchronized (lock) {
                    frames.clear();
                    frames.putAll(next);
                }
                AndroidUtilities.runOnUIThread(() ->
                        NotificationCenter.getGlobalInstance().postNotificationName(NotificationCenter.dialogsNeedReload));
            } catch (Throwable ignore) {
                // Offline or the registry is temporarily down: keep the last
                // known frames instead of dropping every ring.
            } finally {
                if (connection != null) {
                    connection.disconnect();
                }
                inFlight = false;
                handler.postDelayed(DeltagramFrames::refresh, REFRESH_INTERVAL);
            }
        }, "DeltaGramFrames").start();
    }

    /**
     * Paints the ring for a user over an avatar that occupies {@code bounds}.
     * Called both from AvatarDrawable (when the placeholder is visible) and
     * from BackupImageView (once a real photo covers the placeholder).
     */
    public static void paint(Canvas canvas, Rect bounds, long userId) {
        if (canvas == null || bounds == null || bounds.width() < 12) {
            return;
        }
        String kind = frameFor(userId);
        if (kind == null) {
            return;
        }
        paint(canvas, bounds, kind);
    }

    public static void paint(Canvas canvas, Rect bounds, String kind) {
        if (canvas == null || bounds == null || TextUtils.isEmpty(kind) || bounds.width() < 12) {
            return;
        }
        canvas.save();
        canvas.translate(bounds.left, bounds.top);
        try {
            paintKind(canvas, bounds.width(), bounds.height(), kind);
        } catch (Throwable ignored) {
        }
        canvas.restore();
    }

    private static void paintKind(Canvas canvas, int width, int height, String rawKind) {
        String kind = rawKind.trim().toLowerCase();
        float size = Math.min(width, height);
        float cx = width / 2f;
        float cy = height / 2f;
        float radius = size / 2f - Math.max(1f, size * 0.012f);
        float stroke = Math.max(1.8f, size * 0.055f);

        switch (kind) {
            case "scratch":
                paintScratch(canvas, cx, cy, radius, stroke);
                break;
            case "bite":
            case "chewed":
                paintBite(canvas, cx, cy, radius, stroke);
                break;
            case "ember":
                paintEmber(canvas, cx, cy, radius, stroke);
                break;
            case "neon":
                paintNeon(canvas, cx, cy, radius, stroke);
                break;
            case "gold":
                paintGold(canvas, cx, cy, radius, stroke);
                break;
            case "sakura":
                paintSakura(canvas, cx, cy, radius, stroke);
                break;
            case "void":
                paintVoid(canvas, cx, cy, radius, stroke);
                break;
            case "frost":
                paintFrost(canvas, cx, cy, radius, stroke);
                break;
            case "rainbow":
                paintRainbow(canvas, cx, cy, radius, stroke);
                break;
            case "delta":
                paintDelta(canvas, cx, cy, radius, stroke);
                break;
            default:
                break;
        }
    }

    private static void ring(Canvas canvas, float cx, float cy, float radius, Paint p) {
        canvas.drawCircle(cx, cy, radius, p);
    }

    private static Paint stroke(float width, int color) {
        paint.reset();
        paint.setAntiAlias(true);
        paint.setStyle(Paint.Style.STROKE);
        paint.setStrokeWidth(width);
        paint.setStrokeCap(Paint.Cap.ROUND);
        paint.setColor(color);
        return paint;
    }

    private static void glow(Canvas canvas, float cx, float cy, float radius, float width, int color) {
        Paint p = stroke(width * 2.4f, color);
        p.setAlpha(80);
        ring(canvas, cx, cy, radius, p);
    }

    private static void paintScratch(Canvas canvas, float cx, float cy, float radius, float width) {
        Paint p = stroke(width, 0xFF3A3E44);
        ring(canvas, cx, cy, radius, p);

        canvas.save();
        path.reset();
        path.setFillType(Path.FillType.EVEN_ODD);
        path.addCircle(cx, cy, radius + width / 2f, Path.Direction.CW);
        path.addCircle(cx, cy, Math.max(0.1f, radius - width / 2f), Path.Direction.CW);
        canvas.clipPath(path);
        Paint scratch = stroke(Math.max(0.8f, width * 0.22f), 0x96F2F4F8);
        for (int i = 0; i < 9; i++) {
            float angle = (float) (i * 37f + 11f) * (float) Math.PI / 180f;
            float dx = (float) Math.cos(angle);
            float dy = (float) Math.sin(angle);
            float length = radius * (0.9f + 0.1f * (i % 3));
            canvas.drawLine(cx - dx * length, cy - dy * length, cx + dx * length, cy + dy * length, scratch);
        }
        canvas.restore();
    }

    /**
     * A chewed/scratched avatar edge: dark bite holes break the rim, pale tooth
     * arcs sit around them and thin claw marks cut across the photo.
     */
    private static void paintBite(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFFFF4A4A);

        Paint rim = stroke(width, 0xFF1A1014);
        ring(canvas, cx, cy, radius, rim);

        final float[] angles = {18f, 92f, 158f, 226f, 298f, 344f};
        Paint bite = new Paint(Paint.ANTI_ALIAS_FLAG);
        bite.setStyle(Paint.Style.FILL);
        bite.setColor(0xE61A1014);
        Paint tooth = stroke(Math.max(1.2f, width * 0.55f), 0xFFF2E6DC);
        rect.set(cx - radius, cy - radius, cx + radius, cy + radius);
        for (int i = 0; i < angles.length; i++) {
            final double a = Math.toRadians(angles[i]);
            final float bx = cx + (float) Math.cos(a) * radius;
            final float by = cy + (float) Math.sin(a) * radius;
            final float br = width * (i % 2 == 0 ? 1.05f : 0.75f);
            canvas.drawCircle(bx, by, br, bite);
            canvas.drawArc(rect, angles[i] - 14f, 28f, false, tooth);
        }

        Paint claw = stroke(Math.max(1f, width * 0.20f), 0xB3FFFFFF);
        for (int i = 0; i < 7; i++) {
            final float angle = (float) (i * 49f + 12f) * (float) Math.PI / 180f;
            final float dx = (float) Math.cos(angle);
            final float dy = (float) Math.sin(angle);
            final float inner = radius * (0.70f + 0.06f * (i % 3));
            canvas.drawLine(cx + dx * inner, cy + dy * inner,
                    cx + dx * (radius + width * 0.2f), cy + dy * (radius + width * 0.2f), claw);
        }
        Paint shadow = stroke(Math.max(0.8f, width * 0.12f), 0x66000000);
        for (int i = 0; i < 7; i++) {
            final float angle = (float) (i * 49f + 15f) * (float) Math.PI / 180f;
            final float dx = (float) Math.cos(angle);
            final float dy = (float) Math.sin(angle);
            final float inner = radius * (0.72f + 0.06f * (i % 3));
            canvas.drawLine(cx + dx * inner, cy + dy * inner + width * 0.12f,
                    cx + dx * (radius + width * 0.18f), cy + dy * (radius + width * 0.18f) + width * 0.12f, shadow);
        }
    }

    private static void paintEmber(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFFFF6E28);
        Paint p = stroke(width, Color.WHITE);
        p.setShader(new SweepGradient(cx, cy, new int[]{
                0xFFFFD678, 0xFFFF7A2C, 0xFFD63020, 0xFF78181E, 0xFFFFD678
        }, new float[]{0f, 0.30f, 0.60f, 0.85f, 1f}));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);
        p = stroke(Math.max(0.8f, width * 0.16f), 0x78FFECC8);
        ring(canvas, cx, cy, radius - width * 0.55f, p);
    }

    private static void paintNeon(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFF5ADCFF);
        Paint p = stroke(width, Color.WHITE);
        p.setShader(new SweepGradient(cx, cy, new int[]{
                0xFF40FFF0, 0xFF5082FF, 0xFFFF4ED6, 0xFF40FFF0
        }, new float[]{0f, 0.45f, 0.75f, 1f}));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);
        p = stroke(Math.max(0.7f, width * 0.14f), 0x96FFFFFF);
        ring(canvas, cx, cy, radius - width * 0.8f, p);
    }

    private static void paintDelta(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFF7A1F2B);
        Paint p = stroke(width, 0xFFB03A3A);
        ring(canvas, cx, cy, radius, p);
        float br = radius * 0.34f;
        float bx = cx + radius * 0.64f;
        float by = cy + radius * 0.64f;
        Paint fill = new Paint(Paint.ANTI_ALIAS_FLAG);
        fill.setStyle(Paint.Style.FILL);
        fill.setColor(0xFF9E2A2A);
        canvas.drawCircle(bx, by, br, fill);
        Paint border = stroke(Math.max(1.5f, radius * 0.055f), 0xFFFFFFFF);
        border.setStyle(Paint.Style.STROKE);
        canvas.drawCircle(bx, by, br, border);
        Paint text = new Paint(Paint.ANTI_ALIAS_FLAG);
        text.setColor(Color.WHITE);
        text.setTypeface(Typeface.create(Typeface.DEFAULT, Typeface.BOLD));
        text.setTextSize(br * 1.45f);
        text.setTextAlign(Paint.Align.CENTER);
        Paint.FontMetrics fm = text.getFontMetrics();
        canvas.drawText("D", bx, by - (fm.ascent + fm.descent) / 2f, text);
    }

    private static void paintGold(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFFFFCE54);
        Paint p = stroke(width, Color.WHITE);
        p.setShader(new LinearGradient(cx - radius, cy - radius, cx + radius, cy + radius, new int[]{
                0xFFFFF9CD, 0xFFFFD65E, 0xFFC48C20, 0xFFFFE284, 0xFF966614
        }, new float[]{0f, 0.30f, 0.55f, 0.80f, 1f}, Shader.TileMode.CLAMP));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);
        p = stroke(Math.max(0.9f, width * 0.20f), 0xBEFFFFEE);
        rect.set(cx - radius, cy - radius, cx + radius, cy + radius);
        canvas.drawArc(rect, 35f, 70f, false, p);
    }

    private static void paintSakura(Canvas canvas, float cx, float cy, float radius, float width) {
        radius = radius - Math.max(1f, radius * 0.018f);
        Paint p = stroke(Math.max(1.4f, width * 0.7f), Color.WHITE);
        p.setShader(new SweepGradient(cx, cy, new int[]{
                0xFFFFD6E6, 0xFFFF84B0, 0xFFFFD6E6
        }, new float[]{0f, 0.5f, 1f}));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);

        paint.reset();
        paint.setAntiAlias(true);
        paint.setStyle(Paint.Style.FILL);
        paint.setColor(0xD7FFB2CC);
        float rw = width * 0.85f;
        float rh = width * 1.45f;
        for (int i = 0; i < 8; i++) {
            canvas.save();
            canvas.rotate(i * 45f + (i % 2 != 0 ? 16f : 0f), cx, cy);
            canvas.drawOval(cx - rw, cy - radius - rh, cx + rw, cy - radius + rh, paint);
            canvas.restore();
        }
    }

    private static void paintVoid(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFF8252E6);
        Paint p = stroke(width, Color.WHITE);
        p.setStrokeCap(Paint.Cap.BUTT);
        p.setShader(new SweepGradient(cx, cy, new int[]{
                0xFFA878FF, 0xFF422078, 0xFF140C26, 0xFFA878FF
        }, new float[]{0f, 0.40f, 0.70f, 1f}));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);

        paint.reset();
        paint.setAntiAlias(true);
        paint.setStyle(Paint.Style.FILL);
        paint.setColor(0xBEBE9CFF);
        for (int i = 0; i < 3; i++) {
            canvas.save();
            canvas.rotate(120f * i + 24f, cx, cy);
            path.reset();
            path.moveTo(cx, cy - radius - width * 0.1f);
            path.lineTo(cx + width * 0.55f, cy - radius + width * 1.5f);
            path.lineTo(cx - width * 0.55f, cy - radius + width * 1.5f);
            path.close();
            canvas.drawPath(path, paint);
            canvas.restore();
        }
    }

    private static void paintFrost(Canvas canvas, float cx, float cy, float radius, float width) {
        radius = radius - Math.max(1f, radius * 0.008f);
        glow(canvas, cx, cy, radius, width, 0xFF8CDCFF);
        Paint p = stroke(width, Color.WHITE);
        p.setShader(new LinearGradient(cx - radius, cy - radius, cx + radius, cy + radius, new int[]{
                0xFFE2FAFF, 0xFF7ECEFF, 0xFF266AC4
        }, new float[]{0f, 0.45f, 1f}, Shader.TileMode.CLAMP));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);

        paint.reset();
        paint.setAntiAlias(true);
        paint.setStyle(Paint.Style.FILL);
        paint.setColor(0xD2ECFCFF);
        for (int i = 0; i < 6; i++) {
            canvas.save();
            canvas.rotate(i * 60f + 15f, cx, cy);
            path.reset();
            path.moveTo(cx, cy - radius + width * 0.2f);
            path.lineTo(cx + width * 0.45f, cy - radius + width * 1.7f);
            path.lineTo(cx - width * 0.45f, cy - radius + width * 1.7f);
            path.close();
            canvas.drawPath(path, paint);
            canvas.restore();
        }
    }

    private static void paintRainbow(Canvas canvas, float cx, float cy, float radius, float width) {
        glow(canvas, cx, cy, radius, width, 0xFFFFFFFF);
        Paint p = stroke(width, Color.WHITE);
        p.setShader(new SweepGradient(cx, cy, new int[]{
                0xFFFF5C5C, 0xFFFFC454, 0xFF78F08C, 0xFF54DCFF,
                0xFF6E82FF, 0xFFD678FF, 0xFFFF5C5C
        }, new float[]{0f, 0.16f, 0.33f, 0.50f, 0.66f, 0.83f, 1f}));
        ring(canvas, cx, cy, radius, p);
        p.setShader(null);
        p = stroke(Math.max(0.8f, width * 0.12f), 0xA0FFFFFF);
        ring(canvas, cx, cy, radius - width * 0.75f, p);
    }
}
