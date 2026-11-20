package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.animation.ValueAnimator;
import android.annotation.SuppressLint;
import android.content.Context;
import android.view.MotionEvent;
import android.view.VelocityTracker;
import android.view.View;
import android.view.ViewConfiguration;
import android.view.ViewGroup;
import android.widget.FrameLayout;

import org.telegram.ui.Components.CubicBezierInterpolator;

/**
 * Card pager from StaticGram's Money screen: neighbours peek from the sides,
 * shrink slightly and fade while the selected card stays crisp. It is a plain
 * FrameLayout with translated children, so it works with the stock Telegram
 * {@link WalletCardComponent} without any StaticGram backend.
 */
@SuppressLint("ViewConstructor")
public class SGMoneyCardsPager extends FrameLayout {

    public interface Listener {
        void onPageSelected(int index);

        void onScroll(float position);
    }

    private static final float ALPHA = 0.45f;
    private static final float SCALE = 0.1f;

    private final View[] pages;
    private final int touchSlop;
    private final int minFling;

    private ValueAnimator animator;
    private VelocityTracker tracker;

    private float position;
    private int selected;
    private boolean dragging;
    private boolean released;
    private float downX;
    private float downY;
    private float dragOriginX;
    private float lastDownX;
    private float startPosition;

    private Listener listener;

    public SGMoneyCardsPager(Context context, View... views) {
        super(context);
        setClipChildren(false);
        setClipToPadding(false);
        pages = views;
        for (View v : views) {
            addView(v, new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT));
        }
        touchSlop = ViewConfiguration.get(context).getScaledTouchSlop();
        minFling = dp(350);
        updatePageMotion();
    }

    public void setListener(Listener listener) {
        this.listener = listener;
    }

    public int getSelected() {
        return selected;
    }

    public float getPosition() {
        return position;
    }

    public boolean isDragging() {
        return dragging;
    }

    private int pageWidth(int totalWidth) {
        return Math.max(1, totalWidth - dp(36));
    }

    private float spacing() {
        return ((getWidth() / 2f) - dp(22)) + (((pageWidth(getWidth()) - dp(24)) * 0.9f) / 2f);
    }

    private static float rubber(float value) {
        return (0.25f * value) / (Math.abs(value) + 1f);
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        final int width = MeasureSpec.getSize(widthMeasureSpec);
        final int cardWidth = pageWidth(width);
        int height = 0;
        for (View page : pages) {
            page.measure(
                    MeasureSpec.makeMeasureSpec(cardWidth, MeasureSpec.EXACTLY),
                    MeasureSpec.makeMeasureSpec(0, MeasureSpec.UNSPECIFIED));
            height = Math.max(height, page.getMeasuredHeight());
        }
        setMeasuredDimension(width, height);
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        final int width = right - left;
        for (View page : pages) {
            final int pageW = page.getMeasuredWidth();
            final int x = (width - pageW) / 2;
            page.layout(x, 0, x + pageW, page.getMeasuredHeight());
        }
        applyTransforms();
    }

    private void applyTransforms() {
        final float spacing = spacing();
        for (int i = 0; i < pages.length; i++) {
            final View page = pages[i];
            final float delta = i - position;
            final float min = Math.min(1f, Math.abs(delta));
            page.setTranslationX(delta * spacing);
            final float scale = 1f - SCALE * min;
            page.setScaleX(scale);
            page.setScaleY(scale);
            page.setAlpha(1f - ALPHA * min);
            page.setLayerType(i == selected || min <= 0f ? LAYER_TYPE_NONE : LAYER_TYPE_HARDWARE, null);
        }
    }

    private void setPosition(float value) {
        position = value;
        final int next = Math.max(0, Math.min(pages.length - 1, Math.round(value)));
        final boolean changed = next != selected;
        selected = next;
        if (changed) {
            updatePageMotion();
        }
        applyTransforms();
        if (listener != null) {
            listener.onScroll(Math.max(0f, Math.min(pages.length - 1, value)));
        }
        if (changed && listener != null) {
            listener.onPageSelected(next);
        }
    }

    public void setPage(int index, boolean animated) {
        final int target = Math.max(0, Math.min(pages.length - 1, index));
        if (animator != null) {
            animator.cancel();
            animator = null;
        }
        if (!animated || getWidth() <= 0) {
            setPosition(target);
            return;
        }
        final float from = position;
        final float to = target;
        animator = ValueAnimator.ofFloat(from, to);
        animator.setDuration(Math.max(220L, (long) (Math.min(1f, Math.abs(to - from)) * 420L)));
        animator.setInterpolator(CubicBezierInterpolator.EASE_OUT_QUINT);
        animator.addUpdateListener(a -> setPosition((float) a.getAnimatedValue()));
        animator.start();
    }

    private void updatePageMotion() {
        for (int i = 0; i < pages.length; i++) {
            if (pages[i] instanceof WalletCardComponent) {
                ((WalletCardComponent) pages[i]).setUseGyroscope(i == selected ? 1f : 0f);
            }
        }
    }

    private int indexForPosition(float value, float velocity) {
        if (Math.abs(velocity) > minFling) {
            return velocity < 0 ? (int) Math.ceil(value - 0.15f) : (int) Math.floor(value + 0.15f);
        }
        return Math.round(value);
    }

    /**
     * The card itself consumes taps and tilts. Handling horizontal swipes here,
     * before the child, makes the pager work from any point on the card.
     */
    @Override
    public boolean dispatchTouchEvent(MotionEvent ev) {
        if (tracker == null) {
            tracker = VelocityTracker.obtain();
        }
        tracker.addMovement(ev);
        switch (ev.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downX = ev.getX();
                downY = ev.getY();
                lastDownX = downX;
                dragOriginX = position;
                dragging = false;
                released = false;
                return super.dispatchTouchEvent(ev);
            case MotionEvent.ACTION_MOVE: {
                final float dx = ev.getX() - downX;
                final float dy = ev.getY() - downY;
                if (!dragging) {
                    if (Math.abs(dx) <= touchSlop || Math.abs(dx) <= Math.abs(dy)) {
                        return super.dispatchTouchEvent(ev);
                    }
                    dragging = true;
                    dragOriginX = position;
                    MotionEvent cancel = MotionEvent.obtain(ev);
                    cancel.setAction(MotionEvent.ACTION_CANCEL);
                    super.dispatchTouchEvent(cancel);
                    cancel.recycle();
                    if (getParent() != null) {
                        getParent().requestDisallowInterceptTouchEvent(true);
                    }
                }
                lastDownX = ev.getX();
                float next = dragOriginX - dx / Math.max(1f, spacing());
                final float max = pages.length - 1;
                if (next < 0) next = -rubber(-next);
                else if (next > max) next = max + rubber(next - max);
                setPosition(next);
                return true;
            }
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL: {
                if (!dragging) {
                    recycleTracker();
                    return super.dispatchTouchEvent(ev);
                }
                dragging = false;
                final float totalDx = ev.getX() - downX;
                float velocity = 0f;
                if (tracker != null) {
                    tracker.computeCurrentVelocity(1000);
                    velocity = tracker.getXVelocity();
                }
                final float max = pages.length - 1;
                final int width = Math.max(1, getWidth());
                int target;
                if (ev.getActionMasked() == MotionEvent.ACTION_UP
                        && (Math.abs(totalDx) > width * 0.12f || Math.abs(velocity) > 380f)) {
                    target = selected + (totalDx < 0 ? 1 : -1);
                } else {
                    target = Math.round(Math.max(0f, Math.min(max, position)));
                }
                target = Math.max(0, Math.min((int) max, target));
                setPage(target, true);
                recycleTracker();
                if (getParent() != null) {
                    getParent().requestDisallowInterceptTouchEvent(false);
                }
                return true;
            }
        }
        return super.dispatchTouchEvent(ev);
    }

    @Override
    public boolean onInterceptTouchEvent(MotionEvent ev) {
        switch (ev.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downX = ev.getX();
                downY = ev.getY();
                lastDownX = downX;
                dragOriginX = position;
                dragging = false;
                released = false;
                startPosition = position;
                if (tracker != null) {
                    tracker.recycle();
                }
                tracker = VelocityTracker.obtain();
                tracker.addMovement(ev);
                if (animator != null) {
                    animator.cancel();
                    animator = null;
                }
                break;
            case MotionEvent.ACTION_MOVE: {
                if (tracker != null) {
                    tracker.addMovement(ev);
                }
                final float dx = ev.getX() - downX;
                final float dy = ev.getY() - downY;
                if (!dragging && Math.abs(dx) > touchSlop && Math.abs(dx) > Math.abs(dy)) {
                    dragging = true;
                    dragOriginX = position;
                    if (getParent() != null) {
                        getParent().requestDisallowInterceptTouchEvent(true);
                    }
                    return true;
                }
                break;
            }
        }
        return false;
    }

    @Override
    public boolean onTouchEvent(MotionEvent ev) {
        if (tracker == null) {
            tracker = VelocityTracker.obtain();
        }
        tracker.addMovement(ev);
        switch (ev.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downX = ev.getX();
                downY = ev.getY();
                lastDownX = downX;
                dragOriginX = position;
                dragging = false;
                released = false;
                startPosition = position;
                return true;
            case MotionEvent.ACTION_MOVE: {
                final float dx = ev.getX() - downX;
                final float dy = ev.getY() - downY;
                if (!dragging) {
                    if (Math.abs(dx) <= touchSlop || Math.abs(dx) <= Math.abs(dy)) {
                        return true;
                    }
                    dragging = true;
                    dragOriginX = position;
                    if (getParent() != null) {
                        getParent().requestDisallowInterceptTouchEvent(true);
                    }
                }
                lastDownX = ev.getX();
                float next = dragOriginX - dx / Math.max(1f, spacing());
                final float max = pages.length - 1;
                if (next < 0) {
                    next = -rubber(-next);
                } else if (next > max) {
                    next = max + rubber(next - max);
                }
                setPosition(next);
                return true;
            }
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL: {
                if (!dragging) {
                    released = true;
                    recycleTracker();
                    return false;
                }
                dragging = false;
                final float totalDx = ev.getX() - downX;
                float velocity = 0f;
                if (tracker != null) {
                    tracker.computeCurrentVelocity(1000);
                    velocity = tracker.getXVelocity();
                }
                final float max = pages.length - 1;
                final int width = Math.max(1, getWidth());
                int target;
                if (ev.getActionMasked() == MotionEvent.ACTION_UP
                        && (Math.abs(totalDx) > width * 0.12f || Math.abs(velocity) > 380f)) {
                    target = selected + (totalDx < 0 ? 1 : -1);
                } else {
                    target = Math.round(Math.max(0f, Math.min(max, position)));
                }
                target = Math.max(0, Math.min((int) max, target));
                setPage(target, true);
                recycleTracker();
                if (getParent() != null) {
                    getParent().requestDisallowInterceptTouchEvent(false);
                }
                return true;
            }
        }
        return super.onTouchEvent(ev);
    }

    private void recycleTracker() {
        if (tracker != null) {
            tracker.recycle();
            tracker = null;
        }
    }

    @Override
    public boolean canScrollHorizontally(int direction) {
        return direction < 0 ? position > 0.01f : position < pages.length - 1 - 0.01f;
    }

    @Override
    protected void onDetachedFromWindow() {
        super.onDetachedFromWindow();
        if (animator != null) {
            animator.cancel();
            animator = null;
        }
        recycleTracker();
    }
}
