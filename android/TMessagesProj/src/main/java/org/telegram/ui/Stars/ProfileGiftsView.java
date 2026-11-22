package org.telegram.ui.Stars;

import static org.telegram.messenger.AndroidUtilities.dp;
import static org.telegram.messenger.AndroidUtilities.dpf2;
import static org.telegram.messenger.AndroidUtilities.lerp;
import static org.telegram.ui.Stars.StarsController.findAttribute;
import static org.telegram.ui.Stars.StarsIntroActivity.setGiftImage;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.RadialGradient;
import android.graphics.RectF;
import android.graphics.Shader;
import android.os.SystemClock;
import android.view.HapticFeedbackConstants;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewParent;

import androidx.annotation.NonNull;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.ApplicationLoader;
import org.telegram.messenger.ImageReceiver;
import org.telegram.messenger.MessagesController;
import org.telegram.messenger.NotificationCenter;
import org.telegram.messenger.UserConfig;
import org.telegram.messenger.Utilities;
import org.telegram.messenger.browser.Browser;
import org.telegram.tgnet.TLRPC;
import org.telegram.tgnet.tl.TL_stars;
import org.telegram.ui.ActionBar.ActionBar;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.AnimatedFloat;
import org.telegram.ui.Components.ButtonBounce;
import org.telegram.ui.Components.CubicBezierInterpolator;
import org.telegram.ui.ProfileActivity;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Random;

public class ProfileGiftsView extends View implements NotificationCenter.NotificationCenterDelegate {

    private final int currentAccount;
    private final long dialogId;
    private final View avatarContainer;
    private final ProfileActivity.AvatarImageView avatarImage;
    private final Theme.ResourcesProvider resourcesProvider;

    // Free positioning of pinned gifts ("bubbles"). Positions are stored locally
    // per account / profile / gift, because the MTProto API has no free placement.
    private static final long DRAG_LONG_PRESS_MS = 350;
    private static final String PREFS_NAME = "profile_gifts_positions";
    private final boolean canDrag;

    private static final java.util.ArrayList<java.lang.ref.WeakReference<ProfileGiftsView>> INSTANCES = new java.util.ArrayList<>();
    private final Paint skinRingPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint skinGlowPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint bubbleFillPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint bubbleRimPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint bubbleHighlightPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint bubbleGlyphPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF bubbleRimRect = new RectF();

    /** Positions are re-generated from a stable seed, like Telegram for iOS does. */
    private final Random bubbleRandom = new Random(System.currentTimeMillis() / 1000L);
    private boolean positionsDirty = true;
    private int positionsWidth, positionsHeight;
    private float positionsExpandY, positionsCx, positionsCy;
    private final long animationsStart = SystemClock.uptimeMillis();

    /** Repaints every visible profile after the user picks another bubble skin. */
    public static void notifySkinChanged() {
        for (int i = INSTANCES.size() - 1; i >= 0; i--) {
            final ProfileGiftsView view = INSTANCES.get(i).get();
            if (view == null) {
                INSTANCES.remove(i);
            } else {
                view.invalidate();
            }
        }
    }

    private Gift draggingGift;
    private float dragCx, dragCy;
    private float dragTouchOffsetX, dragTouchOffsetY;
    private float downX, downY;
    private boolean dragLongPressPosted;
    private final Runnable dragLongPressRunnable = this::startDrag;

    /** Size of the avatar box used for the last layout, for normalised offsets. */
    private float lastAvatarWidth, lastAvatarHeight;

    private boolean active = true;
    public void setActive(boolean f) {
        this.active = f;
    }

    public ProfileGiftsView(Context context, int currentAccount, long dialogId, @NonNull View avatarContainer, ProfileActivity.AvatarImageView avatarImage, Theme.ResourcesProvider resourcesProvider) {
        super(context);

        this.currentAccount = currentAccount;
        this.dialogId = dialogId;

        this.avatarContainer = avatarContainer;
        this.avatarImage = avatarImage;

        this.resourcesProvider = resourcesProvider;

        this.canDrag = dialogId == UserConfig.getInstance(currentAccount).getClientUserId();
        INSTANCES.add(new java.lang.ref.WeakReference<>(this));
    }

    public float expandProgress, collapseProgress;
    public boolean isOpening;

    public void setExpandProgress(float progress) {
        if (this.expandProgress != progress) {
            this.expandProgress = progress;
            invalidate();
        }
    }

    public void setCollapseProgress(float progress, boolean isOpening) {
        this.isOpening = isOpening;
        float newProgress = Utilities.clamp01((progress - 0.3f) / 0.7f);
        if (this.collapseProgress != newProgress) {
            this.collapseProgress = newProgress;
            invalidate();
        }
    }

    private float actionBarProgress;

    public void setActionBarActionMode(float progress) {
//        if (Theme.isCurrentThemeDark()) {
//            return;
//        }
        actionBarProgress = progress;
        invalidate();
    }


    private float left, right, cy;
    private float expandY, maxExpandY;
    private final AnimatedFloat rightAnimated = new AnimatedFloat(this, 0, 350, CubicBezierInterpolator.EASE_OUT_QUINT);

    public void setBounds(float left, float right, float cy, boolean animated, int maxExpandY) {
        boolean changed = Math.abs(left - this.left) > 0.1f || Math.abs(right - this.right) > 0.1f || Math.abs(cy - this.cy) > 0.1f;
        this.left = left;
        this.right = right;
        if (!animated) {
            this.rightAnimated.set(this.right, true);
        }
        this.cy = cy;
        this.maxExpandY = maxExpandY + cy;
        if (changed) {
            invalidate();
        }
    }

    public void setExpandCoords(float y) {
        this.expandY = y;
        invalidate();
    }

    private float progressToInsets = 1f;

    public void setProgressToStoriesInsets(float progressToInsets) {
        if (this.progressToInsets == progressToInsets) {
            return;
        }
        this.progressToInsets = progressToInsets;
        invalidate();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();

        NotificationCenter.getInstance(currentAccount).addObserver(this, NotificationCenter.starUserGiftsLoaded);

        for (Gift gift : gifts) {
            if (gift.imageReceiver != null) {
                gift.imageReceiver.onAttachedToWindow();
            }
        }

        update();
    }

    @Override
    protected void onDetachedFromWindow() {
        super.onDetachedFromWindow();

        NotificationCenter.getInstance(currentAccount).removeObserver(this, NotificationCenter.starUserGiftsLoaded);

        cancelDragLongPress();
        finishDrag(false);

        for (Gift gift : gifts) {
            if (gift.imageReceiver != null) {
                gift.imageReceiver.onDetachedFromWindow();
            }
        }
    }

    @Override
    public void didReceivedNotification(int id, int account, Object... args) {
        if (id == NotificationCenter.starUserGiftsLoaded) {
            if ((long) args[0] == dialogId) {
                update();
            }
        }
    }

    private SharedPreferences prefs() {
        return ApplicationLoader.applicationContext == null ? null
                : ApplicationLoader.applicationContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
    }

    private String positionKey(long giftId) {
        return currentAccount + "_" + dialogId + "_" + giftId;
    }

    private void loadPosition(Gift gift) {
        final SharedPreferences prefs = prefs();
        if (prefs == null) {
            return;
        }
        final String value = prefs.getString(positionKey(gift.id), null);
        if (value == null) {
            return;
        }
        final int comma = value.indexOf(',');
        if (comma <= 0) {
            return;
        }
        try {
            final float x = Float.parseFloat(value.substring(0, comma));
            final float y = Float.parseFloat(value.substring(comma + 1));
            if (!Float.isNaN(x) && !Float.isInfinite(x) && !Float.isNaN(y) && !Float.isInfinite(y)) {
                // Sanity clamp: a corrupt/infinite offset used to push every
                // bubble off-screen and made the whole collection disappear.
                gift.offsetX = Math.max(-2f, Math.min(2f, x));
                gift.offsetY = Math.max(-2f, Math.min(2f, y));
            } else {
                prefs.edit().remove(positionKey(gift.id)).apply();
            }
        } catch (NumberFormatException ignore) {
        }
    }

    private void savePosition(Gift gift) {
        final SharedPreferences prefs = prefs();
        if (prefs == null) {
            return;
        }
        prefs.edit()
                .putString(positionKey(gift.id), gift.offsetX + "," + gift.offsetY)
                .apply();
    }

    public final class Gift {

        public final long id;
        /** Base gift id of a collectible, used to find the generic artwork. */
        private final long giftId;
        public TLRPC.Document document;
        public long documentId;
        public final int color;
        public final String slug;
        private StarsReactionsSheet.Particles particles;
        private boolean documentResolved;

        public int position = -1;

        public Gift(TL_stars.TL_starGiftUnique gift) {
            id = gift.id;
            giftId = gift.gift_id;
            TLRPC.Document doc = gift.getDocument();
            if (doc != null && doc.id == 0) {
                doc = null;
            }
            document = doc;
            documentId = doc == null ? 0 : doc.id;
            final TL_stars.starGiftAttributeBackdrop backdrop = findAttribute(gift.attributes, TL_stars.starGiftAttributeBackdrop.class);
            color = backdrop.center_color | 0xFF000000;
            slug = gift.slug;
            initParticles();
        }

        public Gift(TLRPC.TL_emojiStatusCollectible status) {
            id = status.collectible_id;
            giftId = 0;
            document = null;
            documentId = status.document_id;
            color = status.center_color | 0xFF000000;
            slug = status.slug;
            initParticles();
        }

        private void initParticles() {
            particles = new StarsReactionsSheet.Particles(StarsReactionsSheet.Particles.TYPE_RADIAL, 5);
            final float gsz = dp(46);
            particles.bounds.set(-gsz / 2.0f, -gsz / 2.0f, gsz / 2.0f, gsz / 2.0f);
        }

        public boolean equals(Gift b) {
            return b != null && b.id == id;
        }

        public RadialGradient gradient;
        public final Matrix gradientMatrix = new Matrix();
        public Paint gradientPaint;
        public ImageReceiver imageReceiver;
        public AnimatedFloat animatedFloat;

        public final RectF bounds = new RectF();
        public final ButtonBounce bounce = new ButtonBounce(ProfileGiftsView.this);

        /** User offset from the default slot position, normalised to the avatar box. */
        public float offsetX, offsetY;
        /** Default slot position of the last draw, without the user offset. */
        public float baseCx, baseCy;
        /** Last drawn center, including the user offset. */
        public float lastCx, lastCy;

        /** Orbit slot around the avatar, in the style of Telegram for iOS. */
        public float orbitDistance, orbitAngle, orbitScale = 1f;
        /** Idle float/wiggle/glow animation parameters for this bubble. */
        public float idlePhase, hoverDuration = 4f, wiggleDuration = 2.5f, glowDuration = 2.5f;

        public void copy(Gift b) {
            gradient = b.gradient;
            gradientPaint = b.gradientPaint;
            animatedFloat = b.animatedFloat;
            particles = b.particles;
            position = b.position;
            offsetX = b.offsetX;
            offsetY = b.offsetY;
            orbitDistance = b.orbitDistance;
            orbitAngle = b.orbitAngle;
            orbitScale = b.orbitScale;
            idlePhase = b.idlePhase;
            hoverDuration = b.hoverDuration;
            wiggleDuration = b.wiggleDuration;
            glowDuration = b.glowDuration;
            if (documentId == 0 && b.documentId != 0) {
                // Keep the artwork that was already resolved for this gift.
                document = b.document;
                documentId = b.documentId;
                documentResolved = true;
            }
            if (b.imageReceiver != null && b.documentId == documentId) {
                // Reuse the already running sticker animation instead of
                // recreating an ImageReceiver on every list refresh.
                imageReceiver = b.imageReceiver;
                imageReceiver.setParentView(ProfileGiftsView.this);
            } else if (b.imageReceiver != null) {
                b.imageReceiver.onDetachedFromWindow();
                b.imageReceiver = null;
            }
        }

        /** Starts loading the gift artwork. Models are lottie documents on this server. */
        public void ensureImageReceiver() {
            if (imageReceiver != null || document == null || document.id == 0) {
                return;
            }
            imageReceiver = new ImageReceiver(ProfileGiftsView.this);
            setGiftImage(imageReceiver, document, 160);
            if (isAttachedToWindow()) {
                imageReceiver.onAttachedToWindow();
            }
        }

        /**
         * Some saved gifts arrive without a model attribute document. Fall back
         * to the generic gift artwork of the same base gift, so the bubble never
         * ends up as an empty coloured circle.
         */
        public void resolveDocument() {
            if (documentId != 0 || documentResolved || giftId == 0) {
                return;
            }
            documentResolved = true;
            final StarsController starsController = StarsController.getInstance(currentAccount);
            final TL_stars.StarGift base = starsController.getStarGift(giftId);
            if (base != null && base.sticker != null) {
                applyDocument(base.sticker);
            } else {
                starsController.getStarGift(giftId, gift -> {
                    if (gift != null) {
                        applyDocument(gift.sticker);
                    }
                });
            }
        }

        private void applyDocument(TLRPC.Document doc) {
            if (doc == null || doc.id == 0 || documentId != 0) {
                return;
            }
            document = doc;
            documentId = doc.id;
            ensureImageReceiver();
            invalidate();
        }

        public void release() {
            if (imageReceiver != null) {
                imageReceiver.onDetachedFromWindow();
                imageReceiver = null;
            }
            gradient = null;
        }

        public void draw(
                Canvas canvas,
                float cx, float cy,
                float ascale, float rotate,
                float alpha,
                float glowAlpha
        ) {
            if (alpha <= 0.0f) return;
            final float d = dp(54);
            final float hit = dp(60);
            bounds.set(cx - hit / 2f, cy - hit / 2f, cx + hit / 2f, cy + hit / 2f);
            canvas.save();
            canvas.translate(cx, cy);
            canvas.rotate(rotate);
            final float scale = ascale * bounce.getScale(0.1f);
            canvas.scale(scale, scale);

            final int skin = DeltamentBubbleSkins.get();
            final boolean defaultSkin = DeltamentBubbleSkins.isDefault(skin);
            final int accent = defaultSkin ? color : DeltamentBubbleSkins.accent(skin);
            final float radius = d / 2f;
            final boolean dark = Theme.isCurrentThemeDark();

            // Soft coloured glow behind the sphere. Telegram for iOS draws a
            // 44pt tinted radial shadow behind its 32pt icon.
            if (gradientPaint != null) {
                gradientPaint.setAlpha((int) ((defaultSkin ? 0x58 : 0x78) * alpha * glowAlpha));
                canvas.drawCircle(0, 0, dpf2(34), gradientPaint);
            }

            // Sphere body: a smoky tint, mostly transparent, like the soap
            // bubbles in the reference video.
            bubbleFillPaint.setShader(null);
            bubbleFillPaint.setStyle(Paint.Style.FILL);
            bubbleFillPaint.setColor(accent);
            bubbleFillPaint.setAlpha((int) ((dark ? 0x1E : 0x12) * alpha));
            canvas.drawCircle(0, 0, radius, bubbleFillPaint);

            // Thin light rim, with a brighter crescent at the top and a faint
            // reflected arc at the bottom.
            final float rimRadius = radius - dp(0.6f);
            bubbleRimPaint.setStyle(Paint.Style.STROKE);
            bubbleRimPaint.setStrokeWidth(dp(1.05f));
            bubbleRimPaint.setColor(dark ? 0xFFFFFFFF : 0xFF000000);
            bubbleRimPaint.setAlpha((int) ((dark ? 0.30f : 0.20f) * 0xFF * alpha));
            canvas.drawCircle(0, 0, rimRadius, bubbleRimPaint);
            bubbleRimRect.set(-rimRadius, -rimRadius, rimRadius, rimRadius);
            bubbleRimPaint.setAlpha((int) ((dark ? 0.72f : 0.34f) * 0xFF * alpha));
            canvas.drawArc(bubbleRimRect, 152f, 122f, false, bubbleRimPaint);
            bubbleRimPaint.setAlpha((int) ((dark ? 0.22f : 0.12f) * 0xFF * alpha));
            canvas.drawArc(bubbleRimRect, -18f, 62f, false, bubbleRimPaint);

            // Outer accent glow for the coloured skins.
            if (!defaultSkin) {
                skinGlowPaint.setStyle(Paint.Style.STROKE);
                skinGlowPaint.setStrokeWidth(dp(5f));
                skinGlowPaint.setColor(accent);
                skinGlowPaint.setAlpha((int) (60 * alpha));
                canvas.drawCircle(0, 0, radius, skinGlowPaint);
                skinRingPaint.setStyle(Paint.Style.STROKE);
                skinRingPaint.setStrokeWidth(dp(DeltamentBubbleSkins.ringWidth(skin)));
                skinRingPaint.setColor(accent);
                skinRingPaint.setAlpha((int) (220 * alpha));
                canvas.drawCircle(0, 0, radius - dp(1f), skinRingPaint);
            }

            particles.process();
            particles.draw(canvas, dark ? 0xFFFFFFFF : accent, alpha);

            if (imageReceiver != null) {
                final float sz = dp(28);
                imageReceiver.setImageCoords(-sz / 2f, -sz / 2f, sz, sz);
                imageReceiver.setAlpha(alpha);
                imageReceiver.draw(canvas);
            } else {
                bubbleGlyphPaint.setStyle(Paint.Style.FILL);
                bubbleGlyphPaint.setColor(dark ? 0xFFFFFFFF : 0xFF000000);
                bubbleGlyphPaint.setAlpha((int) (0.7f * alpha * 0xFF));
                bubbleGlyphPaint.setTextSize(dp(22));
                bubbleGlyphPaint.setTextAlign(Paint.Align.CENTER);
                final Paint.FontMetrics fm = bubbleGlyphPaint.getFontMetrics();
                canvas.drawText("\u2605", 0, -(fm.ascent + fm.descent) / 2f, bubbleGlyphPaint);
            }

            // Tiny specular dot on the upper left, the detail that reads as glass.
            bubbleHighlightPaint.setStyle(Paint.Style.FILL);
            bubbleHighlightPaint.setColor(0xFFFFFFFF);
            bubbleHighlightPaint.setAlpha((int) (0x66 * alpha));
            canvas.drawCircle(-radius * 0.42f, -radius * 0.50f, radius * 0.045f, bubbleHighlightPaint);

            canvas.restore();
        }
    }

    private StarsController.GiftsList list;

    public final ArrayList<Gift> oldGifts = new ArrayList<>();
    public final ArrayList<Gift> gifts = new ArrayList<>();
    public final HashSet<Long> giftIds = new HashSet<>();
    public int maxCount;

    public void update() {
        // DeltaGram: keep the bubbles independent from the server-side
        // "gifts in profile" flag. On this server the flag is not always
        // present, and a stale/false value used to hide the whole collection.
        if (draggingGift != null) {
            finishDrag(true);
        }

        maxCount = MessagesController.getInstance(currentAccount).stargiftsPinnedToTopLimit;
        if (maxCount <= 0) {
            // A missing/zero server value used to produce an empty bubble list.
            maxCount = 6;
        }
        oldGifts.clear();
        oldGifts.addAll(gifts);
        gifts.clear();
        giftIds.clear();

        final TLRPC.EmojiStatus emojiStatus;
        if (dialogId >= 0) {
            final TLRPC.User user = MessagesController.getInstance(currentAccount).getUser(dialogId);
            emojiStatus = user == null ? null : user.emoji_status;
        } else {
            final TLRPC.User chat = MessagesController.getInstance(currentAccount).getUser(-dialogId);
            emojiStatus = chat == null ? null : chat.emoji_status;
        }
        if (emojiStatus instanceof TLRPC.TL_emojiStatusCollectible) {
            giftIds.add(((TLRPC.TL_emojiStatusCollectible) emojiStatus).collectible_id);
        }
        list = StarsController.getInstance(currentAccount).getProfileGiftsList(dialogId);
        if (list != null) {
            for (int i = 0; i < list.gifts.size(); i++) {
                final TL_stars.SavedStarGift savedGift = list.gifts.get(i);
                if (!savedGift.unsaved && savedGift.pinned_to_top && savedGift.gift instanceof TL_stars.TL_starGiftUnique) {
                    final Gift gift = new Gift((TL_stars.TL_starGiftUnique) savedGift.gift);
                    if (!giftIds.contains(gift.id)) {
                        gifts.add(gift);
                        giftIds.add(gift.id);
                    }
                }
            }
            // DeltaGram: every collectible gift becomes a bubble, on any
            // profile. Only the owner can drag them, but viewers should still
            // see the collection instead of an empty header. Pinned gifts keep
            // their priority (they are added first above).
            for (int i = 0; i < list.gifts.size() && gifts.size() < maxCount; i++) {
                final TL_stars.SavedStarGift savedGift = list.gifts.get(i);
                if (!savedGift.unsaved && savedGift.gift instanceof TL_stars.TL_starGiftUnique) {
                    final Gift gift = new Gift((TL_stars.TL_starGiftUnique) savedGift.gift);
                    if (!giftIds.contains(gift.id)) {
                        gifts.add(gift);
                        giftIds.add(gift.id);
                    }
                }
            }
        }

        boolean changed = false;
        if (gifts.size() != oldGifts.size()) {
            changed = true;
        } else for (int i = 0; i < gifts.size(); i++) {
            if (!gifts.get(i).equals(oldGifts.get(i))) {
                changed = true;
                break;
            }
        }

        for (int i = 0; i < gifts.size(); i++) {
            final Gift g = gifts.get(i);
            Gift oldGift = null;
            for (int j = 0; j < oldGifts.size(); ++j) {
                if (oldGifts.get(j).id == g.id) {
                    oldGift = oldGifts.get(j);
                    break;
                }
            }

            if (oldGift != null) {
                g.copy(oldGift);
            } else {
                // Soft halo behind the glass bubble, using the gift's own
                // backdrop colour (the same source iOS uses for its glow).
                g.gradient = new RadialGradient(0, 0, dp(34), new int[]{g.color, Theme.multAlpha(g.color, 0.0f)}, new float[]{0, 1}, Shader.TileMode.CLAMP);
                g.gradientPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
                g.gradientPaint.setShader(g.gradient);
                g.animatedFloat = new AnimatedFloat(this, 0, 320, null);
                g.animatedFloat.force(0.0f);
                // Idle float parameters, staggered like the iOS cover does.
                g.idlePhase = (gifts.indexOf(g) * 0.37f) + bubbleRandom.nextFloat() * 2.5f;
                g.hoverDuration = 3.5f + bubbleRandom.nextFloat();
                g.wiggleDuration = 2f + bubbleRandom.nextFloat();
                g.glowDuration = 2f + bubbleRandom.nextFloat();
                loadPosition(g);
            }
            g.resolveDocument();
            g.ensureImageReceiver();
        }

        List<Integer> positions = new ArrayList<>();
        for (int i = 0; i < maxCount; i++) {
            positions.add(i);
        }

        for (int i = 0; i < oldGifts.size(); i++) {
            final Gift g = oldGifts.get(i);
            Gift newGift = null;
            for (int j = 0; j < gifts.size(); ++j) {
                if (gifts.get(j).id == g.id) {
                    newGift = gifts.get(j);
                    break;
                }
            }
            if (newGift == null) {
                g.release();
            } else {
                positions.remove((Object) g.position);
            }
        }

        if (!positions.isEmpty()) {
            BagRandomizer<Integer> bagRandomizer = new BagRandomizer<>(positions);
            for (int i = 0; i < gifts.size(); ++i) {
                final Gift g = gifts.get(i);
                if (g.position == -1) {
                    g.position = bagRandomizer.next();
                }
            }
        }

        if (changed || positionsWidth != getWidth() || positionsHeight != getHeight()) {
            positionsDirty = true;
            invalidate();
        }
    }

    @Override
    protected void dispatchDraw(@NonNull Canvas canvas) {
        if (!active || gifts.isEmpty()) return;

        final float realX = avatarContainer.getX();
        final float realY = avatarContainer.getY();
        final float realW = avatarContainer.getWidth() * avatarContainer.getScaleX();
        final float realH = avatarContainer.getHeight() * avatarContainer.getScaleY();
        final float barBottom = AndroidUtilities.statusBarHeight + ActionBar.getCurrentActionBarHeight();
        final float acx = realW > 0f ? realX + realW / 2f : getWidth() / 2f;
        final float acy = realH > 0f ? realY + realH / 2f : barBottom + dp(54);

        // setExpandCoords() is only called from the header layout pass, so on
        // the very first frames expandY can still be 0. Never let that hide the
        // whole collection.
        final float headerBottom = expandY > barBottom + dp(40)
                ? expandY
                : Math.max(realY + realH + dp(168), barBottom + dp(168));
        final float closedAlpha = Utilities.clamp01((headerBottom - barBottom) / dp(50));
        // Keep a small hint of the bubbles even in the expanded photo state,
        // and never fade them out completely at rest.
        final float visibility = closedAlpha * (1f - 0.85f * expandProgress) * (1f - actionBarProgress);
        if (visibility <= 0.001f) return;

        final float avatarSize = dp(ProfileActivity.isProfileV2Enabled() ? 108 : 96);
        if (positionsDirty || positionsWidth != getWidth() || positionsHeight != getHeight()
                || Math.abs(positionsExpandY - headerBottom) > 0.5f
                || Math.abs(positionsCx - acx) > dp(40) || Math.abs(positionsCy - acy) > dp(40)) {
            computePositions(acx, acy, avatarSize, headerBottom);
        }

        lastAvatarWidth = avatarSize;
        lastAvatarHeight = avatarSize;

        canvas.save();
        canvas.clipRect(0, 0, getWidth(), headerBottom);

        final float t = (SystemClock.uptimeMillis() - animationsStart) / 1000f;

        for (int i = 0; i < gifts.size(); ++i) {
            final Gift gift = gifts.get(i);
            final float enter = gift.animatedFloat != null ? gift.animatedFloat.set(1.0f) : 1f;
            final float phase = gift.idlePhase;
            final float hover = (float) Math.sin((t / gift.hoverDuration + phase) * Math.PI * 2.0) * dpf2(1.6f);
            final float wiggle = (float) Math.sin((t / gift.wiggleDuration + phase * 1.7f) * Math.PI * 2.0) * 0.04f;
            final float glow = 0.8f + 0.2f * (float) Math.sin((t / gift.glowDuration + phase * 0.6f) * Math.PI * 2.0);

            float gx = acx + (float) Math.cos(gift.orbitAngle) * gift.orbitDistance;
            float gy = acy + (float) Math.sin(gift.orbitAngle) * gift.orbitDistance + hover;
            gift.baseCx = gx;
            gift.baseCy = gy;

            final boolean hasOffset = (gift.offsetX != 0f || gift.offsetY != 0f)
                    && !Float.isNaN(gift.offsetX) && !Float.isInfinite(gift.offsetX)
                    && !Float.isNaN(gift.offsetY) && !Float.isInfinite(gift.offsetY);
            if (hasOffset) {
                gx += gift.offsetX * avatarSize;
                gy += gift.offsetY * avatarSize;
                // Keep custom positions on-screen after rotation / resize.
                gx = clamp(gx, dragPad(), Math.max(dragPad(), getWidth() - dragPad()));
                gy = clamp(gy, dragPad(), Math.max(dragPad(), headerBottom - dragPad()));
            }

            // Fly out of the avatar on first appearance, like the iOS cover.
            gx = lerp(acx, gx, enter);
            gy = lerp(acy, gy, enter);
            final float scale = gift.orbitScale * lerp(0.2f, 1f, enter);

            gift.lastCx = gx;
            gift.lastCy = gy;

            if (gift == draggingGift) {
                continue;
            }
            gift.draw(canvas, gx, gy, scale, wiggle, visibility * enter, glow);
        }

        if (draggingGift != null) {
            draggingGift.draw(canvas, dragCx, dragCy, 1.15f, 0f, 1f, 1.2f);
        }

        canvas.restore();

        if (isAttachedToWindow()) {
            postInvalidateOnAnimation();
        }
    }

    /**
     * Port of the orbit layout from Telegram for iOS
     * (PeerInfoGiftsCoverComponent.PositionGenerator): the first four bubbles
     * sit on a tight orbit around the avatar, the rest on an outer one, while
     * the status bar, navigation buttons and the title stay clear.
     */
    private void computePositions(float acx, float acy, float avatarSize, float headerBottom) {
        final int count = gifts.size();
        if (count <= 0) {
            positionsDirty = false;
            return;
        }

        final float w = Math.max(getWidth(), 1);
        final float h = Math.max(headerBottom, dp(140));
        final float radius = avatarSize / 2f;
        final float item = dp(48);
        final float pad = dp(5);
        final float minDistance = dp(52);

        final float barTop = AndroidUtilities.statusBarHeight;
        final float barBottom = barTop + ActionBar.getCurrentActionBarHeight();

        final ArrayList<RectF> exclusions = new ArrayList<>();
        // Status bar plus the navigation buttons (gifts may float between them,
        // exactly like on iOS).
        exclusions.add(new RectF(0, 0, w, barTop + dp(4)));
        exclusions.add(new RectF(dp(4), barTop, dp(60), barBottom));
        exclusions.add(new RectF(w - dp(150), barTop, w - dp(4), barBottom));
        // Name and status under the avatar.
        final float titleTop = acy + radius + dp(4);
        final float titleWidth = Math.min(w - dp(24), dp(300));
        exclusions.add(new RectF((w - titleWidth) / 2f, titleTop, (w + titleWidth) / 2f, titleTop + dp(64)));
        // Action capsules (message / call ...) at the bottom of the header.
        exclusions.add(new RectF(0, h - dp(78), w, h));

        final float[] distances = new float[count];
        final float[] angles = new float[count];
        final float[] scales = new float[count];
        int placed = 0;
        int left = 0, right = 0;
        int attempts = 0;

        final int innerCount = Math.min(4, count);
        final float maxPossibleDistance = (float) Math.hypot(w, h) / 2f;

        while (placed < innerCount && attempts < count * 250) {
            attempts++;
            final boolean onLeft = right > left;
            final float distance = (1.4f + 0.8f * bubbleRandom.nextFloat()) * radius;
            final float angle = (onLeft ? (float) Math.PI / 2f : -(float) Math.PI / 2f)
                    + (float) Math.PI * bubbleRandom.nextFloat();
            final float px = acx + (float) Math.cos(angle) * distance;
            final float py = acy + (float) Math.sin(angle) * distance;
            if (!isPositionFree(px, py, item, pad, w, h, exclusions, distances, angles, placed, acx, acy, minDistance)) {
                continue;
            }
            distances[placed] = distance;
            angles[placed] = angle;
            scales[placed] = 0.85f + 0.18f * bubbleRandom.nextFloat();
            placed++;
            if (px < acx) left++; else right++;
        }

        while (placed < count && attempts < count * 600) {
            attempts++;
            final boolean onLeft = right >= left;
            final float distance = (2.5f + 1.1f * bubbleRandom.nextFloat()) * radius;
            final float angle = (onLeft ? (float) Math.PI / 2f : -(float) Math.PI / 2f)
                    + (float) Math.PI * bubbleRandom.nextFloat();
            final float px = acx + (float) Math.cos(angle) * distance;
            final float py = acy + (float) Math.sin(angle) * distance;
            if (!isPositionFree(px, py, item, pad, w, h, exclusions, distances, angles, placed, acx, acy, minDistance)) {
                continue;
            }
            distances[placed] = distance;
            angles[placed] = angle;
            scales[placed] = 1.05f - Math.min(1f, distance / Math.max(1f, maxPossibleDistance)) * 0.16f;
            placed++;
            if (px < acx) left++; else right++;
        }

        // Fallback for very narrow covers: spread the remaining bubbles on the
        // sides / lower half, never over the action bar.
        int guard = 0;
        while (placed < count && guard++ < count * 4) {
            final float angle = (float) (Math.PI * (placed + 0.5f) / Math.max(1, count));
            distances[placed] = radius * 1.75f;
            angles[placed] = angle;
            scales[placed] = 0.9f;
            placed++;
        }

        for (int i = 0; i < count; i++) {
            final Gift gift = gifts.get(i);
            gift.orbitDistance = distances[i];
            gift.orbitAngle = angles[i];
            gift.orbitScale = scales[i];
        }

        positionsWidth = getWidth();
        positionsHeight = getHeight();
        positionsExpandY = headerBottom;
        positionsCx = acx;
        positionsCy = acy;
        positionsDirty = false;
    }

    private boolean isPositionFree(
            float px, float py, float item, float pad, float w, float h,
            ArrayList<RectF> exclusions,
            float[] distances, float[] angles, int placed,
            float acx, float acy, float minDistance
    ) {
        if (px - item / 2f < pad || px + item / 2f > w - pad
                || py - item / 2f < pad || py + item / 2f > h - pad) {
            return false;
        }
        final float half = item / 2f;
        for (int i = 0; i < exclusions.size(); i++) {
            final RectF zone = exclusions.get(i);
            if (px + half > zone.left && px - half < zone.right
                    && py + half > zone.top && py - half < zone.bottom) {
                return false;
            }
        }
        final float effectiveMinDistance = placed > 5 ? Math.max(minDistance * 0.7f, dp(10)) : minDistance;
        for (int i = 0; i < placed; i++) {
            final float ex = acx + (float) Math.cos(angles[i]) * distances[i];
            final float ey = acy + (float) Math.sin(angles[i]) * distances[i];
            if (Math.hypot(ex - px, ey - py) < effectiveMinDistance) {
                return false;
            }
        }
        return true;
    }

    public Gift getGiftUnder(float x, float y) {
        for (int i = gifts.size() - 1; i >= 0; --i) {
            if (gifts.get(i).bounds.contains(x, y))
                return gifts.get(i);
        }
        return null;
    }

    private Gift pressedGift;

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        if (!active) return false;
        final int action = event.getActionMasked();

        if (action == MotionEvent.ACTION_DOWN) {
            final Gift hit = getGiftUnder(event.getX(), event.getY());
            pressedGift = hit;
            downX = event.getX();
            downY = event.getY();
            if (pressedGift != null) {
                pressedGift.bounce.setPressed(true);
                if (canDrag) {
                    dragLongPressPosted = true;
                    removeCallbacks(dragLongPressRunnable);
                    postDelayed(dragLongPressRunnable, DRAG_LONG_PRESS_MS);
                }
            }
            return pressedGift != null;
        }

        if (draggingGift != null) {
            if (action == MotionEvent.ACTION_MOVE) {
                dragCx = clamp(event.getX() + dragTouchOffsetX, dragPad(), getWidth() - dragPad());
                dragCy = clamp(event.getY() + dragTouchOffsetY, dragPad(), Math.max(dragPad(), headerBottomForDrag() - dragPad()));
                invalidate();
            } else if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
                finishDrag(action == MotionEvent.ACTION_UP);
            }
            return true;
        }

        if (action == MotionEvent.ACTION_MOVE) {
            if (pressedGift != null) {
                final float dx = event.getX() - downX;
                final float dy = event.getY() - downY;
                final float slop = AndroidUtilities.touchSlop;
                if (dx * dx + dy * dy > slop * slop) {
                    cancelDragLongPress();
                    pressedGift.bounce.setPressed(false);
                    pressedGift = null;
                }
            }
        } else if (action == MotionEvent.ACTION_UP) {
            cancelDragLongPress();
            if (pressedGift != null) {
                onGiftClick(pressedGift);
                pressedGift.bounce.setPressed(false);
                pressedGift = null;
            }
        } else if (action == MotionEvent.ACTION_CANCEL) {
            cancelDragLongPress();
            if (pressedGift != null) {
                pressedGift.bounce.setPressed(false);
                pressedGift = null;
            }
        }
        return pressedGift != null;
    }

    private void startDrag() {
        dragLongPressPosted = false;
        if (!canDrag || pressedGift == null || !active) {
            return;
        }
        final Gift gift = pressedGift;
        pressedGift = null;
        draggingGift = gift;
        gift.bounce.setPressed(false);
        dragCx = gift.lastCx;
        dragCy = gift.lastCy;
        dragTouchOffsetX = dragCx - downX;
        dragTouchOffsetY = dragCy - downY;
        final ViewParent parent = getParent();
        if (parent != null) {
            parent.requestDisallowInterceptTouchEvent(true);
        }
        performHapticFeedback(HapticFeedbackConstants.LONG_PRESS);
        invalidate();
    }

    private void finishDrag(boolean apply) {
        final Gift gift = draggingGift;
        draggingGift = null;
        final ViewParent parent = getParent();
        if (parent != null) {
            parent.requestDisallowInterceptTouchEvent(false);
        }
        if (gift != null && apply) {
            if (lastAvatarWidth > 0f && lastAvatarHeight > 0f) {
                final float nx = (dragCx - gift.baseCx) / lastAvatarWidth;
                final float ny = (dragCy - gift.baseCy) / lastAvatarHeight;
                if (!Float.isNaN(nx) && !Float.isInfinite(nx) && !Float.isNaN(ny) && !Float.isInfinite(ny)) {
                    gift.offsetX = Math.max(-2f, Math.min(2f, nx));
                    gift.offsetY = Math.max(-2f, Math.min(2f, ny));
                    savePosition(gift);
                }
            }
        }
        invalidate();
    }

    private void cancelDragLongPress() {
        if (dragLongPressPosted) {
            dragLongPressPosted = false;
            removeCallbacks(dragLongPressRunnable);
        }
    }

    private float dragPad() {
        return dpf2(24);
    }

    /** Bottom of the header area, even before setExpandCoords() has run. */
    private float headerBottomForDrag() {
        final float barBottom = AndroidUtilities.statusBarHeight + ActionBar.getCurrentActionBarHeight();
        return expandY > barBottom + dp(40) ? expandY : Math.max(getHeight(), barBottom + dp(168));
    }

    private static float clamp(float value, float min, float max) {
        return value < min ? min : (value > max ? max : value);
    }

    public void onGiftClick(Gift gift) {
        Browser.openUrl(getContext(), "https://t.me/nft/" + gift.slug);
    }
}
