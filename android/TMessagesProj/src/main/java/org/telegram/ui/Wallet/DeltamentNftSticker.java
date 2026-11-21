package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewConfiguration;
import android.widget.FrameLayout;

import org.telegram.ui.Components.BackupImageView;
import org.telegram.ui.Components.LayoutHelper;

/**
 * A single NFT/gift sticker on the card face. Coordinates are normalised to the
 * card (0..1), exactly like the StaticGram card sticker model, so the same
 * sticker looks identical on every screen size and on other users' devices.
 */
public class DeltamentNftSticker extends FrameLayout {

    private final WalletCardComponent card;
    private final BackupImageView image;
    private final boolean editable;
    private final int touchSlop;

    private String url;
    private float nx = 0.5f;
    private float ny = 0.5f;
    private float scale = 0.231f;
    private float rotation;

    private float downRawX, downRawY;
    private float downX, downY;
    private boolean dragging;

    private Runnable onChanged;

    public DeltamentNftSticker(Context context, WalletCardComponent card, boolean editable) {
        super(context);
        this.card = card;
        this.editable = editable;
        this.touchSlop = ViewConfiguration.get(context).getScaledTouchSlop();
        setClipChildren(false);
        setClipToPadding(false);
        setWillNotDraw(true);
        setClickable(editable);

        image = new BackupImageView(context);
        image.setLayerType(View.LAYER_TYPE_HARDWARE, null);
        image.setImageBitmap(null);
        addView(image, LayoutHelper.createFrame(1, 1));

        FrameLayout face = card.getFrontFace();
        if (face != null) {
            face.addView(this, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));
        }
    }

    public void setOnChanged(Runnable runnable) {
        onChanged = runnable;
    }

    public void setNft(String url, float x, float y, float scale, float rotation) {
        this.url = url;
        this.nx = clamp01(x);
        this.ny = clamp01(y);
        this.scale = Math.max(0.05f, Math.min(1f, scale));
        this.rotation = rotation;
        if (url == null || url.isEmpty()) {
            image.setImageBitmap(null);
            setClickable(false);
        } else {
            image.setImage(url, "256_256", (Drawable) null);
            setClickable(editable);
        }
        requestLayout();
        invalidate();
    }

    public String getUrl() {
        return url;
    }

    public float getNftX() {
        return nx;
    }

    public float getNftY() {
        return ny;
    }

    public float getNftScale() {
        return scale;
    }

    public float getNftRotation() {
        return rotation;
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        super.onLayout(changed, left, top, right, bottom);
        layoutSticker();
    }

    private void layoutSticker() {
        FrameLayout face = card.getFrontFace();
        if (face == null || face.getWidth() <= 0 || face.getHeight() <= 0) {
            return;
        }
        int size = Math.max(dp(18), Math.round(face.getWidth() * scale));
        int cx = Math.round(face.getWidth() * nx);
        int cy = Math.round(face.getHeight() * ny);
        int l = cx - size / 2;
        int t = cy - size / 2;
        image.layout(l, t, l + size, t + size);
        image.setPivotX(size / 2f);
        image.setPivotY(size / 2f);
        image.setRotation(rotation);
        image.setVisibility(url == null || url.isEmpty() ? GONE : VISIBLE);
    }

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        if (!editable || url == null || url.isEmpty()) {
            return false;
        }
        switch (event.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downRawX = event.getRawX();
                downRawY = event.getRawY();
                downX = nx;
                downY = ny;
                dragging = false;
                return true;
            case MotionEvent.ACTION_MOVE: {
                float dx = event.getRawX() - downRawX;
                float dy = event.getRawY() - downRawY;
                if (!dragging && Math.hypot(dx, dy) < touchSlop) {
                    return true;
                }
                dragging = true;
                FrameLayout face = card.getFrontFace();
                if (face != null && face.getWidth() > 0 && face.getHeight() > 0) {
                    nx = clamp01(downX + dx / face.getWidth());
                    ny = clamp01(downY + dy / face.getHeight());
                    layoutSticker();
                }
                return true;
            }
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL:
                if (dragging) {
                    dragging = false;
                    if (onChanged != null) {
                        onChanged.run();
                    }
                }
                return true;
        }
        return true;
    }

    private static float clamp01(float value) {
        return Math.max(0f, Math.min(1f, value));
    }
}
