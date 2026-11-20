package org.telegram.ui.Components;

import static org.telegram.messenger.AndroidUtilities.dp;
import static org.telegram.messenger.AndroidUtilities.dpf2;
import static org.telegram.messenger.LocaleController.getString;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.drawable.Drawable;
import android.view.MotionEvent;
import android.view.View;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.R;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Wallet.GramWallet;

public class ProfileBalanceView extends ProfileMusicView {

    private final Drawable gramIcon;
    private final Drawable starIcon;
    private Text tonText;
    private Text starsText;
    private boolean hasCards;
    private float ejectProgress;
    private View.OnClickListener tonClickListener;
    private View.OnClickListener starsClickListener;

    private final Paint chevronPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path chevron = new Path();

    public ProfileBalanceView(Context context, Theme.ResourcesProvider resourcesProvider) {
        super(context, resourcesProvider);
        gramIcon = context.getResources().getDrawable(R.drawable.profile_balance_gram).mutate();
        starIcon = context.getResources().getDrawable(R.drawable.star_small_inner).mutate();
        starIcon.setColorFilter(new android.graphics.PorterDuffColorFilter(0xffffd45e, android.graphics.PorterDuff.Mode.SRC_IN));
        chevronPaint.setStyle(Paint.Style.STROKE);
        chevronPaint.setStrokeCap(Paint.Cap.ROUND);
        chevronPaint.setStrokeJoin(Paint.Join.ROUND);
        chevronPaint.setStrokeWidth(dpf2(1.16f));
        chevron.moveTo(0, -dpf2(3.33f));
        chevron.lineTo(dpf2(3.16f), 0);
        chevron.lineTo(0, dpf2(3.33f));
        setCards(0, 0);
    }

    public void setCards(long ton, long stars) {
        tonText = new Text(GramWallet.formatGrams(ton), 11, AndroidUtilities.bold());
        starsText = new Text(LocaleController.formatNumber(stars, ' '), 11, AndroidUtilities.bold());
        hasCards = ton > 0 || stars > 0;
        setContentDescription("GRAM " + GramWallet.formatGrams(ton) + ", Stars " + stars);
        invalidate();
    }

    public void setEjectProgress(float progress) {
        if (ejectProgress == progress) return;
        ejectProgress = progress;
        invalidate();
    }

    public void setOnSegmentClickListeners(View.OnClickListener ton, View.OnClickListener stars) {
        tonClickListener = ton;
        starsClickListener = stars;
    }

    @Override
    protected float getContentWidth(float maxWidth) {
        if (tonText == null || starsText == null) return dp(48);
        tonText.ellipsize(-1);
        starsText.ellipsize(-1);
        return dp(4 + 13 + 2) + tonText.getWidth() + dp(5)
                + dp(1) // divider
                + dp(5 + 13 + 2) + starsText.getWidth() + dp(6);
    }

    @Override
    protected void drawContent(Canvas canvas, float alpha) {
        if (tonText == null || starsText == null) return;
        final float cy = getHeight() / 2f;
        final int iconSize = dp(13);
        canvas.translate(0, -dp(20) * ejectProgress);

        canvas.translate(dp(4), 0);
        gramIcon.setBounds(0, (int) cy - iconSize / 2, iconSize, (int) cy - iconSize / 2 + iconSize);
        gramIcon.setAlpha((int) (255 * alpha));
        gramIcon.draw(canvas);
        canvas.translate(iconSize + dp(2), 0);
        tonText.draw(canvas, 0, cy, textColor, alpha);
        canvas.translate(tonText.getWidth() + dp(4.8f), cy);
        chevronPaint.setColor(Theme.multAlpha(textColor, 0.85f * alpha));
        canvas.drawPath(chevron, chevronPaint);

        canvas.translate(dpf2(1.5f), -cy);
        canvas.drawLine(0, cy - dp(5), 0, cy + dp(5), chevronPaint);
        canvas.translate(dpf2(5), 0);

        starIcon.setBounds(0, (int) cy - iconSize / 2, iconSize, (int) cy - iconSize / 2 + iconSize);
        starIcon.setAlpha((int) (255 * alpha));
        starIcon.draw(canvas);
        canvas.translate(iconSize + dp(2), 0);
        starsText.draw(canvas, 0, cy, textColor, alpha);
        canvas.translate(starsText.getWidth() + dp(4.8f), cy);
        canvas.drawPath(chevron, chevronPaint);
    }

    @Override
    public boolean dispatchTouchEvent(MotionEvent event) {
        if (!hasCards) {
            return super.dispatchTouchEvent(event);
        }
        if (event.getAction() == MotionEvent.ACTION_UP) {
            if (event.getX() < getWidth() / 2f) {
                if (tonClickListener != null) tonClickListener.onClick(this);
            } else {
                if (starsClickListener != null) starsClickListener.onClick(this);
            }
            return true;
        }
        return true;
    }
}
