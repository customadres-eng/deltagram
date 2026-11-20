package org.telegram.ui.Components;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.ColorFilter;
import android.graphics.LinearGradient;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.RadialGradient;
import android.graphics.RectF;
import android.graphics.Shader;
import android.graphics.drawable.Drawable;

import androidx.annotation.NonNull;
import androidx.annotation.Nullable;

/**
 * A lightweight "liquid glass" surface: a translucent vertical gradient, a
 * broad specular highlight in the upper-left corner and a bright hairline
 * border. It approximates the iOS 26 glass look without a RenderEffect pass,
 * so it stays smooth on low-end devices.
 */
public class LiquidGlassDrawable extends Drawable {

    private final Paint fill = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint glow = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint border = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF rect = new RectF();
    private final Path clip = new Path();
    private final int tint;

    public LiquidGlassDrawable(int tint) {
        this.tint = tint;
        border.setStyle(Paint.Style.STROKE);
        border.setStrokeWidth(dp(1));
        border.setColor(0x33ffffff);
    }

    @Override
    public void draw(@NonNull Canvas canvas) {
        rect.set(getBounds());
        if (rect.isEmpty()) return;
        float r = dp(22);

        fill.setShader(new LinearGradient(0, rect.top, 0, rect.bottom,
                new int[]{0x66ffffff, 0x22ffffff, 0x0dffffff},
                new float[]{0f, 0.45f, 1f}, Shader.TileMode.CLAMP));
        canvas.drawRoundRect(rect, r, r, fill);

        // A faint tint keeps the glass readable over the dark theme.
        fill.setShader(null);
        fill.setColor(tint);
        canvas.drawRoundRect(rect, r, r, fill);

        glow.setShader(new RadialGradient(rect.left + rect.width() * 0.22f, rect.top + rect.height() * 0.08f,
                Math.max(rect.width(), rect.height()) * 0.85f,
                new int[]{0x40ffffff, 0x10ffffff, 0x00ffffff},
                new float[]{0f, 0.55f, 1f}, Shader.TileMode.CLAMP));
        canvas.drawRoundRect(rect, r, r, glow);

        canvas.drawRoundRect(rect, r, r, border);
    }

    @Override
    public void setAlpha(int alpha) {
        fill.setAlpha(alpha);
        glow.setAlpha(alpha);
        border.setAlpha(alpha);
        invalidateSelf();
    }

    @Override
    public void setColorFilter(@Nullable ColorFilter colorFilter) {
        fill.setColorFilter(colorFilter);
        glow.setColorFilter(colorFilter);
        border.setColorFilter(colorFilter);
        invalidateSelf();
    }

    @Override
    public int getOpacity() {
        return PixelFormat.TRANSLUCENT;
    }
}
