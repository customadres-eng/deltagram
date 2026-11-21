package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.text.TextUtils;
import android.util.TypedValue;
import android.view.Gravity;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramCards;
import org.telegram.messenger.DeltagramCurrency;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.R;
import org.telegram.ui.ActionBar.BottomSheet;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.QRCodeBottomSheet;

/**
 * Telegram-style bottom sheet for another DeltaGram account: the card slides
 * up, can be swiped between GRAM and STARS, and shows both balances below.
 */
public class DeltamentPublicCardSheet extends BottomSheet {

    public DeltamentPublicCardSheet(Context context, int account, DeltagramCards.PublicCard card, int kind, Theme.ResourcesProvider resourcesProvider) {
        super(context, false, resourcesProvider);
        fixNavigationBar();

        LinearLayout content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(20), dp(16), dp(20), dp(24));

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
        content.addView(owner, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 4, 0, 14));

        WalletCardComponent gram = new WalletCardComponent(context);
        gram.setCardHolder(card.name.isEmpty() ? "DELTA" : card.name.toUpperCase());
        gram.setCardNumber("GRAM " + card.userId);
        gram.setDiamondOnCard(true);
        DeltamentCardStyle.apply(gram, card.colorId);
        gram.getBalanceView().setText("GRAM " + WalletCardComponent.formatGrams(card.ton), false);
        gram.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromTon(card.ton), false);
        DeltamentNftSticker gramNft = new DeltamentNftSticker(context, gram, false);
        gramNft.setNft(card.nftUrl(), card.nftX, card.nftY, card.nftScale, card.nftRotation);
        gram.setOnClickListener(v -> showQr(context, card, 0, resourcesProvider));

        WalletCardComponent stars = new WalletCardComponent(context);
        stars.setCardHolder(card.name.isEmpty() ? "DELTA" : card.name.toUpperCase());
        stars.setCardNumber("STAR " + card.userId);
        stars.setDiamondOnCard(false);
        DeltamentCardStyle.apply(stars, 1);
        stars.getBalanceView().setText("★ " + WalletCardComponent.formatStars(card.stars), false);
        stars.getUsdBalanceView().setText("≈ " + DeltagramCurrency.fromStars(card.stars), false);
        stars.setOnClickListener(v -> showQr(context, card, 1, resourcesProvider));

        SGMoneyCardsPager pager = new SGMoneyCardsPager(context, gram, stars);
        pager.setPage(kind == 1 ? 1 : 0, false);
        content.addView(pager, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView balances = new TextView(context);
        balances.setText("GRAM: " + WalletCardComponent.formatGrams(card.ton)
                + "   ·   ⭐ " + WalletCardComponent.formatStars(card.stars));
        balances.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 17);
        balances.setTypeface(AndroidUtilities.bold());
        balances.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText, resourcesProvider));
        balances.setGravity(Gravity.CENTER);
        balances.setPadding(0, dp(16), 0, 0);
        content.addView(balances, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView fiat = new TextView(context);
        fiat.setText("≈ " + DeltagramCurrency.fromTon(card.ton) + "  ·  " + DeltagramCurrency.fromStars(card.stars));
        fiat.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        fiat.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText, resourcesProvider));
        fiat.setGravity(Gravity.CENTER);
        fiat.setPadding(0, dp(4), 0, 0);
        content.addView(fiat, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        setCustomView(content);
        setBackgroundColor(Theme.getColor(Theme.key_dialogBackground, resourcesProvider));
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
