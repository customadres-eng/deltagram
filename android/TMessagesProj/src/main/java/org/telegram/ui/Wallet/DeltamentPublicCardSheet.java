package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.drawable.GradientDrawable;
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

/** Read-only public view of another DeltaGram account's wallet card. */
public class DeltamentPublicCardSheet extends BottomSheet {

    public DeltamentPublicCardSheet(Context context, int account, DeltagramCards.PublicCard card, Theme.ResourcesProvider resourcesProvider) {
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

        FrameLayout previewBox = new FrameLayout(context);
        WalletCardComponent preview = new WalletCardComponent(context);
        preview.setCardHolder(card.name.isEmpty() ? "DELTA" : card.name.toUpperCase());
        preview.setCardNumber("GRAM " + card.userId);
        preview.setDiamondOnCard(true);
        DeltamentCardStyle.apply(preview, card.colorId);
        DeltamentNftSticker nftSticker = new DeltamentNftSticker(context, preview, false);
        nftSticker.setNft(card.nftUrl(), card.nftX, card.nftY, card.nftScale, card.nftRotation);
        preview.setOnClickListener(v -> {
            org.telegram.ui.Components.QRCodeBottomSheet qr = new org.telegram.ui.Components.QRCodeBottomSheet(
                    context, "Карта " + (card.username.isEmpty() ? card.name : "@" + card.username),
                    "tg://user?id=" + card.userId,
                    "Отсканируйте QR-код, чтобы открыть профиль владельца карты в Telegram.", false, resourcesProvider);
            try {
                qr.setCenterImage(R.drawable.deltament_logo);
            } catch (Exception ignored) {
            }
            qr.show();
        });
        previewBox.addView(preview, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
        content.addView(previewBox, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView balances = new TextView(context);
        balances.setText("GRAM: " + GramWallet.formatGrams(card.ton) + "   ·   ⭐ " + LocaleController.formatNumber(card.stars, ' '));
        balances.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
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
}
