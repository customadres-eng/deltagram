package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.text.SpannableString;
import android.text.InputType;
import android.text.TextUtils;
import android.text.SpannableStringBuilder;
import android.text.style.ForegroundColorSpan;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramCards;
import org.telegram.messenger.DeltagramCurrency;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.NotificationCenter;
import org.telegram.messenger.UserConfig;
import org.telegram.ui.ActionBar.AlertDialog;
import org.telegram.ui.ActionBar.BaseFragment;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.BulletinFactory;
import org.telegram.ui.Components.EditTextBoldCursor;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.QRCodeBottomSheet;
import org.telegram.ui.Stars.StarsController;
import org.telegram.ui.Stars.StarsIntroActivity;
import org.telegram.ui.TON.TONIntroActivity;

/**
 * The two real Telegram wallet cards on the DeltaGram Money screen. The blue
 * GRAM card and a second stock {@link WalletCardComponent} whose material is
 * shifted to gold for Stars. Swiping between them uses the same pager motion as
 * StaticGram rather than a plain scroll view.
 */
public class DeltamentMoneyCardsView extends LinearLayout implements NotificationCenter.NotificationCenterDelegate {

    private final int currentAccount;
    private final BaseFragment parent;

    private final SGMoneyCardsPager pager;
    private final LinearLayout dots;
    private final TextView action;
    private TextView currencyView;
    private final View[] dotViews = new View[2];

    private final WalletCardComponent tonCard;
    private final WalletCardComponent starsCard;
    private final DeltamentNftSticker tonNft;

    private int active;
    private long tonBalance;
    private long starsBalance;

    private static final int TON_ACCENT = 0xffe5323f;
    private static final int STAR_ACCENT = 0xffffc247;
    private static final int STAR_GOLD = 0xffffd45e;

    public DeltamentMoneyCardsView(Context context, int account, BaseFragment parent) {
        super(context);
        this.currentAccount = account;
        this.parent = parent;
        setOrientation(VERTICAL);
        setClipChildren(false);
        setClipToPadding(false);
        setPadding(0, dp(4), 0, dp(8));

        tonCard = new WalletCardComponent(context, true);
        tonCard.setCardHolder("DELTA");
        tonCard.setCardNumber("GRAM 1780 " + UserConfig.getInstance(account).getClientUserId());
        tonCard.setDiamondOnCard(true);
        tonCard.setUseGyroscope(0f);
        DeltamentCardStyle.apply(tonCard, 2);
        tonCard.setOnClickListener(v -> showCardQr(false));
        tonNft = new DeltamentNftSticker(context, tonCard, false);

        starsCard = new WalletCardComponent(context, true);
        starsCard.setCardHolder("DELTA");
        starsCard.setCardNumber("STAR 1780 " + UserConfig.getInstance(account).getClientUserId());
        starsCard.setDiamondOnCard(false);
        DeltamentCardStyle.apply(starsCard, 1);
        starsCard.setUseGyroscope(0f);
        starsCard.setOnClickListener(v -> showCardQr(true));

        pager = new SGMoneyCardsPager(context, tonCard, starsCard);
        pager.setListener(new SGMoneyCardsPager.Listener() {
            @Override
            public void onPageSelected(int index) {
                updateActive(index);
            }

            @Override
            public void onScroll(float position) {
                updateDots(position);
            }
        });
        addView(pager, LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT));

        dots = new LinearLayout(context);
        dots.setOrientation(HORIZONTAL);
        dots.setGravity(Gravity.CENTER);
        dots.setPadding(0, dp(8), 0, dp(10));
        for (int i = 0; i < 2; i++) {
            View dot = new View(context);
            GradientDrawable bg = new GradientDrawable();
            bg.setCornerRadius(dp(3));
            bg.setColor(i == 0 ? 0xfffdf4f2 : 0x33ffffff);
            dot.setBackground(bg);
            LinearLayout.LayoutParams dlp = new LinearLayout.LayoutParams(dp(i == 0 ? 18 : 6), dp(6));
            dlp.setMargins(dp(3), 0, dp(3), 0);
            final int page = i;
            dot.setOnClickListener(v -> pager.setPage(page, true));
            dots.addView(dot, dlp);
            dotViews[i] = dot;
        }
        addView(dots, LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT));

        action = new TextView(context);
        action.setGravity(Gravity.CENTER);
        action.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 15);
        action.setTypeface(AndroidUtilities.bold());
        action.setTextColor(Color.WHITE);
        action.setOnClickListener(v -> showSendDialog(active == 0 ? "ton" : "stars"));
        LinearLayout.LayoutParams alp = LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, 48);
        alp.leftMargin = dp(20);
        alp.rightMargin = dp(20);
        addView(action, alp);

        currencyView = new TextView(context);
        currencyView.setGravity(Gravity.CENTER);
        currencyView.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        currencyView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        currencyView.setPadding(dp(12), dp(8), dp(12), dp(8));
        currencyView.setOnClickListener(v -> showCurrencyDialog(context));
        updateCurrencyLabel(currencyView);
        addView(currencyView, LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT, 20, 6, 20, 0));

        addView(createCardSettingsRow(context), LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, 64, 20, 8, 20, 0));
        addView(createDomainsRow(context), LayoutHelper.createLinear(LayoutParams.MATCH_PARENT, 64, 20, 8, 20, 0));

        updateBalances();
        updateActive(0);
    }

    /**
     * Tapping a card shows its blockchain-style QR. The GRAM card prefers the
     * real TON address when the wallet has one; otherwise both cards open the
     * account profile through a tg:// link.
     */
    private void showCardQr(boolean stars) {
        if (parent == null || parent.getParentActivity() == null) {
            return;
        }
        final long userId = UserConfig.getInstance(currentAccount).getClientUserId();
        String address = null;
        try {
            address = GramWallet.getInstance(currentAccount).getAddress();
        } catch (Exception ignored) {
        }
        DeltagramCards.PublicCard own = DeltagramCards.cardFor(userId);
        String deltaAddress = own != null ? own.tonAddress : null;
        final String title;
        final String link;
        final String help;
        if (!stars && !TextUtils.isEmpty(deltaAddress)) {
            title = "TON-кошелёк DeltaGram";
            link = deltaAddress.toLowerCase().endsWith(".ton") ? "ton://transfer/" + deltaAddress : "ton://transfer/" + deltaAddress;
            help = "Адрес карты DeltaGram:\n\n" + deltaAddress;
        } else if (!stars && !TextUtils.isEmpty(address)) {
            title = "TON-кошелёк DeltaGram";
            link = "ton://transfer/" + address;
            help = "Отсканируйте QR-код, чтобы открыть адрес TON-кошелька.\n\n" + address;
        } else {
            title = stars ? "Карта Stars DeltaGram" : "Карта DeltaGram";
            link = "tg://user?id=" + userId;
            help = "Отсканируйте QR-код, чтобы открыть профиль владельца карты в Telegram.";
        }
        QRCodeBottomSheet sheet = new QRCodeBottomSheet(parent.getParentActivity(), title, link, help, false);
        try {
            sheet.setCenterImage(org.telegram.messenger.R.drawable.deltament_logo);
        } catch (Exception ignored) {
        }
        parent.showDialog(sheet);
    }

    private void showSendDialog(String kind) {
        if (parent == null || parent.getParentActivity() == null) return;
        parent.showDialog(new DeltamentSendSheet(parent.getParentActivity(), currentAccount, kind, parent.getResourceProvider(), () -> {
            StarsController.getTonInstance(currentAccount).invalidateBalance();
            StarsController.getInstance(currentAccount).invalidateBalance();
            updateBalances();
        }));
    }

    private final Runnable cardRegistryListener = this::applyOwnCardPrefs;

    private View createCardSettingsRow(Context context) {
        return createActionRow(context, "Настроить карту", "Цвет и NFT · видно всем", v -> {
            if (parent != null) parent.presentFragment(new DeltamentCardSettingsActivity());
        });
    }

    private View createActionRow(Context context, String titleText, String subtitleText, View.OnClickListener click) {
        LinearLayout row = new LinearLayout(context);
        row.setOrientation(HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), 0, dp(14), 0);
        row.setBackground(Theme.createSelectorDrawable(Theme.getColor(Theme.key_listSelector), 3));
        row.setOnClickListener(click);

        LinearLayout texts = new LinearLayout(context);
        texts.setOrientation(VERTICAL);
        TextView title = new TextView(context);
        title.setText(titleText);
        title.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
        title.setTypeface(AndroidUtilities.bold());
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        texts.addView(title, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT));
        TextView subtitle = new TextView(context);
        subtitle.setText(subtitleText);
        subtitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        texts.addView(subtitle, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, 0, 2, 0, 0));
        row.addView(texts, LayoutHelper.createLinear(0, LayoutParams.WRAP_CONTENT, 1f));

        TextView chevron = new TextView(context);
        chevron.setText("›");
        chevron.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 26);
        chevron.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        row.addView(chevron, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT));
        return row;
    }

    private void applyOwnCardPrefs() {
        long ownId = UserConfig.getInstance(currentAccount).getClientUserId();
        DeltagramCards.PublicCard own = DeltagramCards.cardFor(ownId);
        if (own == null) {
            return;
        }
        DeltamentCardStyle.apply(tonCard, own.colorId);
        tonNft.setNft(own.nftUrl(), own.nftX, own.nftY, own.nftScale, own.nftRotation);
        if (!TextUtils.isEmpty(own.tonAddress)) {
            if (own.tonAddress.toLowerCase().endsWith(".ton")) {
                tonCard.setCardNumber(own.tonAddress.toUpperCase());
            } else {
                tonCard.setCardNumber(WalletCardComponent.formatAddress(own.tonAddress.toUpperCase()));
            }
        }
    }

    private View createDomainsRow(Context context) {
        LinearLayout row = new LinearLayout(context);
        row.setOrientation(HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), 0, dp(14), 0);
        row.setBackground(Theme.createSelectorDrawable(Theme.getColor(Theme.key_listSelector), 3));
        row.setOnClickListener(v -> {
            if (parent != null) parent.presentFragment(new DeltamentDomainsActivity());
        });

        LinearLayout texts = new LinearLayout(context);
        texts.setOrientation(VERTICAL);

        TextView title = new TextView(context);
        title.setText("Домены .ton");
        title.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
        title.setTypeface(AndroidUtilities.bold());
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        texts.addView(title, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT));

        TextView subtitle = new TextView(context);
        subtitle.setText("Купить, продать и управлять именами");
        subtitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        texts.addView(subtitle, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, 0, 2, 0, 0));

        row.addView(texts, LayoutHelper.createLinear(0, LayoutParams.WRAP_CONTENT, 1f));

        TextView chevron = new TextView(context);
        chevron.setText("›");
        chevron.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 26);
        chevron.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        row.addView(chevron, LayoutHelper.createLinear(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT));
        return row;
    }

    private static GradientDrawable rounded(int color, int radius) {
        GradientDrawable bg = new GradientDrawable();
        bg.setColor(color);
        bg.setCornerRadius(radius);
        return bg;
    }

    private void updateDots(float position) {
        for (int i = 0; i < dotViews.length; i++) {
            float distance = Math.min(1f, Math.abs(i - position));
            ViewGroup.LayoutParams lp = dotViews[i].getLayoutParams();
            int w = dp(i == Math.round(position) ? 18 : 6);
            if (lp.width != w) {
                lp.width = w;
                dotViews[i].setLayoutParams(lp);
            }
            dotViews[i].setAlpha(1f - 0.6f * distance);
        }
    }

    private void updateActive(int index) {
        active = Math.max(0, Math.min(1, index));
        boolean ton = active == 0;
        action.setText(ton ? "Отправить GRAM" : "Отправить Stars");
        action.setTextColor(ton ? Color.WHITE : 0xff2a1a03);
        action.setBackground(rounded(ton ? TON_ACCENT : STAR_ACCENT, dp(24)));
        updateDots(active);
    }

    public void updateBalances() {
        tonBalance = StarsController.getTonInstance(currentAccount).getBalance().amount;
        starsBalance = StarsController.getInstance(currentAccount).getBalance().amount;

        try {
            String address = GramWallet.getInstance(currentAccount).getAddress();
            if (!TextUtils.isEmpty(address)) {
                tonCard.setCardNumber(WalletCardComponent.formatAddress(address.toUpperCase()));
            }
        } catch (Exception ignored) {
        }

        SpannableStringBuilder gram = new SpannableStringBuilder();
        gram.append(GramWallet.formatGrams(tonBalance)).append(" GRAM");
        tonCard.getBalanceView().setText(gram, false);
        tonCard.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromTon(tonBalance), false);

        SpannableString starIcon = new SpannableString("★");
        starIcon.setSpan(new ForegroundColorSpan(STAR_GOLD), 0, starIcon.length(), 0);
        SpannableStringBuilder stars = new SpannableStringBuilder();
        stars.append(starIcon).append(" ").append(LocaleController.formatNumber(starsBalance, ' ')).append(" STARS");
        starsCard.getBalanceView().setText(stars, false);
        starsCard.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromStars(starsBalance), false);
    }

    private void updateCurrencyLabel(TextView view) {
        if (view == null) return;
        view.setText("Валюта: " + DeltagramCurrency.symbol() + " " + DeltagramCurrency.code() + " · изменить");
    }

    private void showCurrencyDialog(Context context) {
        final CharSequence[] items = new CharSequence[DeltagramCurrency.CODES.length];
        for (int i = 0; i < items.length; i++) {
            items[i] = DeltagramCurrency.SYMBOLS[i] + "  " + DeltagramCurrency.NAMES[i] + " (" + DeltagramCurrency.CODES[i] + ")";
        }
        new org.telegram.ui.ActionBar.AlertDialog.Builder(context)
                .setTitle("Валюта карт")
                .setItems(items, (dialog, which) -> {
                    DeltagramCurrency.setCode(DeltagramCurrency.CODES[which]);
                    updateBalances();
                    updateCurrencyLabel(currencyView);
                })
                .setNegativeButton("Отмена", null)
                .show();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        NotificationCenter.getInstance(currentAccount).addObserver(this, NotificationCenter.starBalanceUpdated);
        NotificationCenter.getInstance(currentAccount).addObserver(this, NotificationCenter.walletUpdate);
        DeltagramCards.addListener(cardRegistryListener);
        DeltagramCards.refreshRegistry(currentAccount);
        try {
            StarsController.getTonInstance(currentAccount).invalidateBalance();
            StarsController.getInstance(currentAccount).invalidateBalance();
        } catch (Exception ignored) {
        }
        updateBalances();
        applyOwnCardPrefs();
    }

    @Override
    protected void onDetachedFromWindow() {
        super.onDetachedFromWindow();
        NotificationCenter.getInstance(currentAccount).removeObserver(this, NotificationCenter.starBalanceUpdated);
        NotificationCenter.getInstance(currentAccount).removeObserver(this, NotificationCenter.walletUpdate);
        DeltagramCards.removeListener(cardRegistryListener);
    }

    @Override
    public void didReceivedNotification(int id, int account, Object... args) {
        if (id == NotificationCenter.starBalanceUpdated || id == NotificationCenter.walletUpdate) {
            updateBalances();
        }
    }
}
