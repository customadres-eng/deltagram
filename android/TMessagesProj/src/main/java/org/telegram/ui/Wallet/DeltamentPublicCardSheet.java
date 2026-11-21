package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.Typeface;
import android.text.TextUtils;
import android.util.TypedValue;
import android.view.Gravity;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramCards;
import org.telegram.messenger.DeltagramCurrency;
import org.telegram.messenger.R;
import org.telegram.ui.ActionBar.BottomSheet;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.QRCodeBottomSheet;

/**
 * Telegram-style bottom sheet for another DeltaGram account. It opens on the
 * card the user tapped, can be swiped GRAM <-> STARS, and always shows both
 * balances underneath.
 */
public class DeltamentPublicCardSheet extends BottomSheet {

    public DeltamentPublicCardSheet(Context context, int account, DeltagramCards.PublicCard card, int kind, Theme.ResourcesProvider resourcesProvider) {
        super(context, false, resourcesProvider);
        fixNavigationBar();

        LinearLayout content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(20), dp(14), dp(20), dp(24));

        TextView title = new TextView(context);
        title.setText("Карта DeltaGram");
        title.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 20);
        title.setTypeface(AndroidUtilities.bold());
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText, resourcesProvider));
        title.setGravity(Gravity.CENTER);
        content.addView(title, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView owner = new TextView(context);
        owner.setText(card.name + (!card.username.isEmpty() ? "  @" + card.username : ""));
        owner.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        owner.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, resourcesProvider));
        owner.setGravity(Gravity.CENTER);
        content.addView(owner, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 4, 0, 12));

        // Two explicit tabs make the GRAM/STARS switch obvious even when the
        // swipe gesture competes with the bottom sheet's own drag handling.
        LinearLayout tabs = new LinearLayout(context);
        tabs.setOrientation(LinearLayout.HORIZONTAL);
        tabs.setGravity(Gravity.CENTER);
        TextView gramTab = makeTab(context, "GRAM", resourcesProvider);
        TextView starsTab = makeTab(context, "STARS", resourcesProvider);
        tabs.addView(gramTab, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 0, 0, 0, 0, 24));
        tabs.addView(starsTab, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        content.addView(tabs, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 0, 0, 10));

        WalletCardComponent gram = new WalletCardComponent(context, true);
        gram.setCardHolder(card.name.isEmpty() ? "DELTA" : card.name.toUpperCase());
        gram.setCardNumber("GRAM " + card.userId);
        gram.setDiamondOnCard(true);
        DeltamentCardStyle.apply(gram, card.colorId);
        gram.getBalanceView().setText("GRAM " + WalletCardComponent.formatGrams(card.ton), false);
        gram.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromTon(card.ton), false);
        DeltamentNftSticker gramNft = new DeltamentNftSticker(context, gram, false);
        gramNft.setNft(card.nftUrl(), card.nftX, card.nftY, card.nftScale, card.nftRotation);
        gram.setOnQrClickListener(v -> showQr(context, card, 0, resourcesProvider));

        WalletCardComponent stars = new WalletCardComponent(context, true);
        stars.setCardHolder(card.name.isEmpty() ? "DELTA" : card.name.toUpperCase());
        stars.setCardNumber("STAR " + card.userId);
        stars.setDiamondOnCard(false);
        DeltamentCardStyle.apply(stars, 1);
        stars.getBalanceView().setText("★ " + WalletCardComponent.formatStars(card.stars), false);
        stars.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromStars(card.stars), false);
        stars.setOnQrClickListener(v -> showQr(context, card, 1, resourcesProvider));

        SGMoneyCardsPager pager = new SGMoneyCardsPager(context, gram, stars);
        pager.setMinimumHeight(dp(200));
        pager.setListener(new SGMoneyCardsPager.Listener() {
            @Override
            public void onPageSelected(int index) {
                styleTabs(gramTab, starsTab, index, resourcesProvider);
            }

            @Override
            public void onScroll(float position) {
            }
        });
        pager.setPage(kind == 1 ? 1 : 0, false);
        styleTabs(gramTab, starsTab, kind == 1 ? 1 : 0, resourcesProvider);
        gramTab.setOnClickListener(v -> pager.setPage(0, true));
        starsTab.setOnClickListener(v -> pager.setPage(1, true));
        content.addView(pager, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        LinearLayout balances = new LinearLayout(context);
        balances.setOrientation(LinearLayout.HORIZONTAL);
        balances.setPadding(0, dp(16), 0, 0);
        balances.setWeightSum(2f);
        balances.addView(makeBalanceColumn(context, "GRAM", WalletCardComponent.formatGrams(card.ton),
                DeltagramCurrency.fromTon(card.ton), resourcesProvider), LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));
        balances.addView(makeBalanceColumn(context, "STARS", WalletCardComponent.formatStars(card.stars),
                DeltagramCurrency.fromStars(card.stars), resourcesProvider), LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));
        content.addView(balances, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        setCustomView(content);
        setBackgroundColor(Theme.getColor(Theme.key_dialogBackground, resourcesProvider));
    }

    private static TextView makeTab(Context context, String text, Theme.ResourcesProvider provider) {
        TextView tab = new TextView(context);
        tab.setText(text);
        tab.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        tab.setGravity(Gravity.CENTER);
        tab.setPadding(dp(6), dp(6), dp(6), dp(6));
        tab.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, provider));
        tab.setTypeface(AndroidUtilities.bold());
        return tab;
    }

    private static void styleTabs(TextView gramTab, TextView starsTab, int selected, Theme.ResourcesProvider provider) {
        int active = Theme.getColor(Theme.key_windowBackgroundWhiteBlackText, provider);
        int inactive = Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, provider);
        gramTab.setTextColor(selected == 0 ? active : inactive);
        starsTab.setTextColor(selected == 1 ? active : inactive);
        gramTab.setAlpha(selected == 0 ? 1f : 0.65f);
        starsTab.setAlpha(selected == 1 ? 1f : 0.65f);
    }

    private static LinearLayout makeBalanceColumn(Context context, String label, String value, String fiat,
                                                   Theme.ResourcesProvider provider) {
        LinearLayout column = new LinearLayout(context);
        column.setOrientation(LinearLayout.VERTICAL);
        column.setGravity(Gravity.CENTER);

        TextView labelView = new TextView(context);
        labelView.setText(label);
        labelView.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        labelView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, provider));
        labelView.setGravity(Gravity.CENTER);
        column.addView(labelView, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView valueView = new TextView(context);
        valueView.setText(value);
        valueView.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 19);
        valueView.setTypeface(Typeface.DEFAULT_BOLD);
        valueView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText, provider));
        valueView.setGravity(Gravity.CENTER);
        column.addView(valueView, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 2, 0, 0));

        TextView fiatView = new TextView(context);
        fiatView.setText("≈ " + fiat);
        fiatView.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        fiatView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, provider));
        fiatView.setGravity(Gravity.CENTER);
        column.addView(fiatView, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
        return column;
    }

    private static void showQr(Context context, DeltagramCards.PublicCard card, int kind, Theme.ResourcesProvider provider) {
        String link;
        String title;
        String help;
        if (kind == 0) {
            link = !TextUtils.isEmpty(card.tonAddress) ? card.tonAddress
                    : (!TextUtils.isEmpty(card.username) ? "https://t.me/" + card.username : "tg://user?id=" + card.userId);
            title = "GRAM · " + (TextUtils.isEmpty(card.username) ? card.name : "@" + card.username);
            help = TextUtils.isEmpty(card.tonAddress)
                    ? "Отсканируйте QR-код, чтобы открыть профиль владельца карты."
                    : "Отсканируйте QR-код, чтобы отправить TON на адрес карты.";
        } else {
            link = !TextUtils.isEmpty(card.username) ? "https://t.me/" + card.username : "tg://user?id=" + card.userId;
            title = "Stars · " + (TextUtils.isEmpty(card.username) ? card.name : "@" + card.username);
            help = "Отсканируйте QR-код, чтобы открыть профиль владельца карты.";
        }
        QRCodeBottomSheet qr = new QRCodeBottomSheet(context, title, link, help, false, provider);
        try {
            qr.setCenterImage(R.drawable.deltament_logo);
        } catch (Exception ignored) {
        }
        qr.show();
    }
}
