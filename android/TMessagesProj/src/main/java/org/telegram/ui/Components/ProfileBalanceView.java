package org.telegram.ui.Components;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RectF;
import android.graphics.drawable.Drawable;
import android.view.MotionEvent;
import android.view.View;
import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.R;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Wallet.WalletCardComponent;

/* loaded from: classes6.dex */
public class ProfileBalanceView extends ProfileMusicView {
    private static final int CHEVRON_ROOM = 9;
    private static final int DEAD_ZONE = 14;
    private static final int GAP = 6;
    private static final int PILL_PAD = 8;
    private final Path chevron;
    private final Paint chevronPaint;
    private int downKind;
    private float ejectProgress;
    private Text gramAmount;
    private final ButtonBounce gramBounce;
    private final Drawable gramColorIcon;
    private final Drawable gramIcon;
    private float lastTouchX;
    private final Drawable starIcon;
    private Text starsAmount;
    private final ButtonBounce starsBounce;

    public float getPressScale() {
        return 1.0f;
    }

    public ProfileBalanceView(Context context, Theme.ResourcesProvider resourcesProvider) {
        super(context, resourcesProvider);
        Paint paint = new Paint(1);
        this.chevronPaint = paint;
        Path path = new Path();
        this.chevron = path;
        this.lastTouchX = -1.0f;
        this.downKind = -1;
        this.gramBounce = new ButtonBounce(this);
        this.starsBounce = new ButtonBounce(this);
        this.gramIcon = context.getResources().getDrawable(R.drawable.profile_balance_gram).mutate();
        this.gramColorIcon = context.getResources().getDrawable(R.drawable.profile_balance_gram_color).mutate();
        this.starIcon = context.getResources().getDrawable(R.drawable.star_small_inner).mutate();
        paint.setStyle(Paint.Style.STROKE);
        paint.setStrokeCap(Paint.Cap.ROUND);
        paint.setStrokeJoin(Paint.Join.ROUND);
        paint.setStrokeWidth(AndroidUtilities.dpf2(1.16f));
        path.moveTo(0.0f, -AndroidUtilities.dpf2(3.33f));
        path.lineTo(AndroidUtilities.dpf2(3.16f), 0.0f);
        path.lineTo(0.0f, AndroidUtilities.dpf2(3.33f));
        setBalance(0L, 0L);
    }

    public void setBalance(long j, long j2) {
        String formatGrams = WalletCardComponent.formatGrams(j);
        String formatStars = WalletCardComponent.formatStars(j2);
        this.gramAmount = new Text(formatGrams, 11.0f, AndroidUtilities.bold());
        this.starsAmount = new Text(formatStars, 11.0f, AndroidUtilities.bold());
        setContentDescription(LocaleController.getString(R.string.StarsBalance) + ": " + formatGrams + " GRAM, " + formatStars + " Stars");
        invalidate();
    }

    private float gramPillEnd() {
        return AndroidUtilities.dp(15.0f) + this.gramAmount.getWidth() + AndroidUtilities.dp(9.0f) + AndroidUtilities.dp(8.0f);
    }

    @Override // org.telegram.ui.Components.ProfileMusicView
    public void buildPillPath(Path path, RectF rectF) {
        float dp = rectF.left + AndroidUtilities.dp(8.0f) + gramPillEnd();
        float height = rectF.height() / 2.0f;
        float f = rectF.left;
        float f2 = rectF.top;
        float f3 = rectF.bottom;
        Path.Direction direction = Path.Direction.CW;
        path.addRoundRect(f, f2, dp, f3, height, height, direction);
        path.addRoundRect(dp + AndroidUtilities.dp(6.0f), rectF.top, rectF.right, rectF.bottom, height, height, direction);
    }

    @Override // org.telegram.ui.Components.ProfileMusicView
    public int getGlassPills(RectF rectF, RectF rectF2, RectF rectF3) {
        float dp = rectF.left + AndroidUtilities.dp(8.0f) + gramPillEnd();
        rectF2.set(rectF.left, rectF.top, dp, rectF.bottom);
        rectF3.set(dp + AndroidUtilities.dp(6.0f), rectF.top, rectF.right, rectF.bottom);
        return 2;
    }

    @Override // org.telegram.ui.Components.ProfileMusicView, android.view.View
    public boolean dispatchTouchEvent(MotionEvent motionEvent) {
        if (motionEvent.getAction() == 0) {
            this.lastTouchX = motionEvent.getX();
            this.downKind = tappedKind();
        }
        boolean dispatchTouchEvent = super.dispatchTouchEvent(motionEvent);
        boolean z = false;
        this.gramBounce.setPressed(dispatchTouchEvent && this.downKind == 0);
        ButtonBounce buttonBounce = this.starsBounce;
        if (dispatchTouchEvent && this.downKind == 1) {
            z = true;
        }
        buttonBounce.setPressed(z);
        return dispatchTouchEvent;
    }

    @Override // org.telegram.ui.Components.ProfileMusicView, android.view.View
    public void onDraw(Canvas canvas) {
        float scale = this.gramBounce.getScale(0.04f);
        float scale2 = this.starsBounce.getScale(0.04f);
        RectF pillRect = getPillRect();
        if ((scale == 1.0f && scale2 == 1.0f) || pillRect == null || pillRect.width() <= 0.0f) {
            super.onDraw(canvas);
            return;
        }
        float dp = pillRect.left + AndroidUtilities.dp(8.0f) + gramPillEnd() + (AndroidUtilities.dp(6.0f) / 2.0f);
        float height = getHeight() / 2.0f;
        canvas.save();
        canvas.clipRect(0.0f, 0.0f, dp, getHeight());
        canvas.scale(scale, scale, (pillRect.left + dp) / 2.0f, height);
        super.onDraw(canvas);
        canvas.restore();
        canvas.save();
        canvas.clipRect(dp, 0.0f, getWidth(), getHeight());
        canvas.scale(scale2, scale2, (dp + pillRect.right) / 2.0f, height);
        super.onDraw(canvas);
        canvas.restore();
    }

    public int tappedKind() {
        RectF pillRect = getPillRect();
        if (pillRect == null || this.lastTouchX < 0.0f) {
            return 0;
        }
        float dp = pillRect.left + AndroidUtilities.dp(8.0f) + gramPillEnd() + (AndroidUtilities.dp(6.0f) / 2.0f);
        if (Math.abs(this.lastTouchX - dp) < AndroidUtilities.dp(14.0f)) {
            return -1;
        }
        return this.lastTouchX < dp ? 0 : 1;
    }

    public boolean getChipBoundsOnScreen(int i, RectF rectF) {
        float[] fArr;
        RectF pillRect = getPillRect();
        if (pillRect == null || pillRect.width() <= 0.0f || pillRect.height() <= 0.0f || !isAttachedToWindow()) {
            return false;
        }
        float dp = pillRect.left + AndroidUtilities.dp(8.0f) + gramPillEnd();
        if (i == 1) {
            fArr = new float[]{Math.min(pillRect.right, dp + AndroidUtilities.dp(6.0f)), pillRect.top, pillRect.right, pillRect.bottom};
        } else {
            fArr = new float[]{pillRect.left, pillRect.top, Math.min(pillRect.right, dp), pillRect.bottom};
        }
        View view = this;
        while (true) {
            view.getMatrix().mapPoints(fArr);
            float left = view.getLeft() - view.getScrollX();
            float top = view.getTop() - view.getScrollY();
            fArr[0] = fArr[0] + left;
            fArr[2] = fArr[2] + left;
            fArr[1] = fArr[1] + top;
            fArr[3] = fArr[3] + top;
            if (!(view.getParent() instanceof View)) {
                break;
            }
            view = (View) view.getParent();
        }
        int[] iArr = new int[2];
        view.getLocationOnScreen(iArr);
        float left2 = iArr[0] - view.getLeft();
        float top2 = iArr[1] - view.getTop();
        rectF.set(Math.min(fArr[0], fArr[2]) + left2, Math.min(fArr[1], fArr[3]) + top2, Math.max(fArr[0], fArr[2]) + left2, Math.max(fArr[1], fArr[3]) + top2);
        return rectF.width() > 0.0f && rectF.height() > 0.0f;
    }

    public void setEjectProgress(float f) {
        if (this.ejectProgress == f) {
            return;
        }
        this.ejectProgress = f;
        invalidate();
    }

    @Override // org.telegram.ui.Components.ProfileMusicView
    public float getContentWidth(float f) {
        this.gramAmount.ellipsize(-1.0f);
        this.starsAmount.ellipsize(-1.0f);
        return AndroidUtilities.dp(15.0f) + AndroidUtilities.dp(9.0f) + AndroidUtilities.dp(22.0f) + AndroidUtilities.dp(15.0f) + AndroidUtilities.dp(9.0f) + this.gramAmount.getWidth() + this.starsAmount.getWidth();
    }

    @Override // org.telegram.ui.Components.ProfileMusicView
    public void drawContent(Canvas canvas, float f) {
        float height = getHeight() / 2.0f;
        canvas.translate(0.0f, (-AndroidUtilities.dp(20.0f)) * this.ejectProgress);
        int dp = AndroidUtilities.dp(13.0f);
        Drawable drawable = (this.hasProfileColor || isOnProfilePhoto()) ? this.gramColorIcon : this.gramIcon;
        int i = ((int) height) - (dp / 2);
        int i2 = i + dp;
        drawable.setBounds(0, i, dp, i2);
        int i3 = (int) (255.0f * f);
        drawable.setAlpha(i3);
        drawable.draw(canvas);
        canvas.translate(AndroidUtilities.dp(2.0f) + dp, 0.0f);
        this.gramAmount.draw(canvas, 0.0f, height, this.textColor, f);
        canvas.save();
        canvas.translate(this.gramAmount.getWidth() + AndroidUtilities.dpf2(4.8f), height);
        float f2 = f * 0.85f;
        this.chevronPaint.setColor(Theme.multAlpha(this.textColor, f2));
        canvas.drawPath(this.chevron, this.chevronPaint);
        canvas.restore();
        canvas.translate(this.gramAmount.getWidth() + AndroidUtilities.dp(9.0f) + AndroidUtilities.dp(22.0f), 0.0f);
        this.starIcon.setBounds(0, i, dp, i2);
        this.starIcon.setAlpha(i3);
        this.starIcon.draw(canvas);
        canvas.translate(dp + AndroidUtilities.dp(2.0f), 0.0f);
        this.starsAmount.draw(canvas, 0.0f, height, this.textColor, f);
        canvas.translate(this.starsAmount.getWidth() + AndroidUtilities.dpf2(4.8f), height);
        this.chevronPaint.setColor(Theme.multAlpha(this.textColor, f2));
        canvas.drawPath(this.chevron, this.chevronPaint);
    }
}
