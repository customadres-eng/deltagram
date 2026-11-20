package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.graphics.drawable.Drawable;
import android.view.Gravity;
import android.view.View;
import android.widget.FrameLayout;

import org.telegram.messenger.R;
import org.telegram.ui.Components.BackupImageView;
import org.telegram.ui.Components.LayoutHelper;

/** Colour palettes and the public NFT sticker for the DeltaGram wallet card. */
public final class DeltamentCardStyle {

    public static final String[] NAMES = {
            "Красная", "Золотая", "Синяя", "Зелёная", "Фиолетовая", "Чёрная"
    };

    // 18 floats in the wallet material format. Palette roles must be positive
    // 0x00RRGGBB values, otherwise the material falls back to stock blue.
    private static final float[][] PALETTES = {
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x00ff8a8a, 0x00e5323f, 0x009e1c2a, 0x00ffd0d0,
                    0x00ff8a8a, 0x00e5323f, 0x009e1c2a, 0x00ffd0d0},
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x00ffd45e, 0x00e0a020, 0x00b8860b, 0x00fff4cc,
                    0x00ffd45e, 0x00e0a020, 0x00b8860b, 0x00fff4cc},
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x0064c8ff, 0x002f7fff, 0x00134fb3, 0x00b3e0ff,
                    0x0064c8ff, 0x002f7fff, 0x00134fb3, 0x00b3e0ff},
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x0084f0a8, 0x0027ae60, 0x00145a34, 0x00c8ffdd,
                    0x0084f0a8, 0x0027ae60, 0x00145a34, 0x00c8ffdd},
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x00d0a0ff, 0x007c4dff, 0x00431f99, 0x00e5d4ff,
                    0x00d0a0ff, 0x007c4dff, 0x00431f99, 0x00e5d4ff},
            {0f, 1f, 1f, 0f, 0f, 0f, 0f, 0f, 0f, 0f,
                    0x006b7482, 0x002b3138, 0x000f1216, 0x009aa4b0,
                    0x006b7482, 0x002b3138, 0x000f1216, 0x009aa4b0},
    };

    private DeltamentCardStyle() {
    }

    public static int previewColor(int colorId) {
        if (colorId < 0 || colorId >= PALETTES.length) {
            return 0xff2f7fff;
        }
        return (int) PALETTES[colorId][10] | 0xff000000;
    }

    public static void apply(WalletCardComponent card, int colorId) {
        if (card == null) return;
        if (colorId < 0 || colorId >= PALETTES.length) {
            card.setStickerStyle(null);
        } else {
            card.setStickerStyle(PALETTES[colorId]);
        }
    }

    public static void applyNft(WalletCardComponent card, String url) {
        if (card == null) return;
        FrameLayout face = card.getFrontFace();
        if (face == null) return;
        View old = face.findViewWithTag("deltagram_card_nft");
        if (old != null) {
            face.removeView(old);
        }
        if (url == null || url.isEmpty()) {
            return;
        }
        BackupImageView image = new BackupImageView(face.getContext());
        image.setTag("deltagram_card_nft");
        image.setImage(url, "140_140", (Drawable) null);
        face.addView(image, LayoutHelper.createFrame(72, 72, Gravity.RIGHT | Gravity.BOTTOM, 0, 0, 22, 20));
    }
}
