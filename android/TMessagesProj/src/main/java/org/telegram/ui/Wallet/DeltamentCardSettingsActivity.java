package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.drawable.GradientDrawable;
import android.util.TypedValue;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramCards;
import org.telegram.messenger.R;
import org.telegram.messenger.UserConfig;
import org.telegram.ui.ActionBar.ActionBar;
import org.telegram.ui.ActionBar.BaseFragment;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.BulletinFactory;
import org.telegram.ui.Components.EditTextBoldCursor;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Stories.recorder.ButtonWithCounterView;

/** Pick the public colour of the DeltaGram wallet card. */
public class DeltamentCardSettingsActivity extends BaseFragment {

    private final int currentAccount = UserConfig.selectedAccount;
    private int colorId = -1;
    private WalletCardComponent preview;
    private LinearLayout colorsRow;
    private ButtonWithCounterView saveButton;
    private EditTextBoldCursor addressEdit;

    @Override
    public View createView(Context context) {
        actionBar.setBackButtonImage(R.drawable.ic_ab_back);
        actionBar.setAllowOverlayTitle(true);
        actionBar.setTitle("Настройка карты");
        actionBar.setActionBarMenuOnItemClick(new ActionBar.ActionBarMenuOnItemClick() {
            @Override
            public void onItemClick(int id) {
                if (id == -1) finishFragment();
            }
        });

        ScrollView scroll = new ScrollView(context);
        scroll.setFillViewport(true);
        scroll.setVerticalScrollBarEnabled(false);
        fragmentView = scroll;

        LinearLayout content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(16), dp(8), dp(16), dp(28));
        scroll.addView(content, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        FrameLayout previewBox = new FrameLayout(context);
        previewBox.setPadding(0, dp(10), 0, dp(18));
        preview = new WalletCardComponent(context);
        preview.setCardHolder("DELTA");
        preview.setCardNumber("GRAM " + UserConfig.getInstance(currentAccount).getClientUserId());
        preview.setDiamondOnCard(true);
        previewBox.addView(preview, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
        content.addView(previewBox, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView note = new TextView(context);
        note.setText("Так карту увидят другие пользователи в вашем профиле. Цвет сохраняется на сервере DeltaGram.");
        note.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        note.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        note.setLineSpacing(dp(2), 1f);
        content.addView(note, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 0, 0, 8));

        content.addView(section(context, "Цвет карты"));
        HorizontalScrollView colorsScroll = new HorizontalScrollView(context);
        colorsScroll.setHorizontalScrollBarEnabled(false);
        colorsRow = new LinearLayout(context);
        colorsRow.setOrientation(LinearLayout.HORIZONTAL);
        colorsScroll.addView(colorsRow);
        content.addView(colorsScroll, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        content.addView(section(context, "Адрес TON"));
        TextView addressNote = new TextView(context);
        addressNote.setText("Адрес, на который владелец отправит TON. Можно указать UQ/EQ-адрес или свой .ton домен — он должен принадлежать вам.");
        addressNote.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        addressNote.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        addressNote.setLineSpacing(dp(2), 1f);
        content.addView(addressNote, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 0, 0, 8));

        addressEdit = new EditTextBoldCursor(context);
        addressEdit.setHint("UQ... или myname.ton");
        addressEdit.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 15);
        addressEdit.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        addressEdit.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        addressEdit.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        addressEdit.setSingleLine(true);
        addressEdit.setPadding(dp(12), 0, dp(12), 0);
        addressEdit.setBackground(Theme.createRoundRectDrawable(dp(12), Theme.getColor(Theme.key_windowBackgroundGray)));
        content.addView(addressEdit, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 44, 0, 0, 0, 8));

        ButtonWithCounterView addressSave = new ButtonWithCounterView(context, resourceProvider).setRound();
        addressSave.setText("Сохранить адрес", false);
        addressSave.setOnClickListener(v -> {
            addressSave.setLoading(true);
            DeltagramCards.setAddress(currentAccount, addressEdit.getText().toString(), (card, error) -> {
                addressSave.setLoading(false);
                if (error != null) {
                    BulletinFactory.of(this).createErrorBulletin(error).show();
                } else {
                    BulletinFactory.of(this).createSuccessBulletin("Адрес карты сохранён").show();
                    if (card != null && card.tonAddress != null && !card.tonAddress.isEmpty()) {
                        preview.setCardNumber(card.tonAddress.toUpperCase());
                    }
                }
            });
        });
        content.addView(addressSave, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 44, 0f, 0, 0, 0));

        saveButton = new ButtonWithCounterView(context, resourceProvider).setRound();
        saveButton.setText("Сохранить", false);
        saveButton.setOnClickListener(v -> save());
        content.addView(saveButton, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 48, 0f, 22, 0, 0));

        buildColors(context);
        DeltagramCards.prefs(currentAccount, (p, error) -> {
            if (p != null) {
                colorId = p.colorId;
                applyPreview();
                updateColorSelection();
            }
        });
        if (addressEdit != null) {
            DeltagramCards.PublicCard own = DeltagramCards.cardFor(UserConfig.getInstance(currentAccount).getClientUserId());
            if (own != null && own.tonAddress != null) {
                addressEdit.setText(own.tonAddress);
            }
        }
        applyPreview();
        return fragmentView;
    }

    private TextView section(Context context, String text) {
        TextView view = new TextView(context);
        view.setText(text);
        view.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        view.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        view.setTypeface(AndroidUtilities.bold());
        view.setPadding(0, dp(18), 0, dp(8));
        return view;
    }

    private void buildColors(Context context) {
        colorsRow.removeAllViews();
        addColorCell(context, -1, "Стандарт");
        for (int i = 0; i < DeltamentCardStyle.NAMES.length; i++) {
            addColorCell(context, i, DeltamentCardStyle.NAMES[i]);
        }
        updateColorSelection();
    }

    private void addColorCell(Context context, int id, String name) {
        LinearLayout cell = new LinearLayout(context);
        cell.setOrientation(LinearLayout.VERTICAL);
        cell.setGravity(Gravity.CENTER);
        cell.setPadding(dp(6), dp(4), dp(6), dp(4));
        cell.setTag(id);

        View circle = new View(context);
        GradientDrawable bg = new GradientDrawable();
        bg.setShape(GradientDrawable.OVAL);
        bg.setColor(id < 0 ? 0xff2f7fff : DeltamentCardStyle.previewColor(id));
        bg.setStroke(dp(2), 0x33ffffff);
        circle.setBackground(bg);
        cell.addView(circle, LayoutHelper.createLinear(38, 38));

        TextView label = new TextView(context);
        label.setText(name);
        label.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 11);
        label.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        cell.addView(label, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 0, 4, 0, 0));
        cell.setOnClickListener(v -> {
            colorId = id;
            applyPreview();
            updateColorSelection();
        });
        colorsRow.addView(cell, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 0, 0, dp(10), 0));
    }

    private void updateColorSelection() {
        if (colorsRow == null) return;
        for (int i = 0; i < colorsRow.getChildCount(); i++) {
            LinearLayout cell = (LinearLayout) colorsRow.getChildAt(i);
            Object tag = cell.getTag();
            boolean selected = tag instanceof Integer && (Integer) tag == colorId;
            View circle = cell.getChildAt(0);
            GradientDrawable bg = new GradientDrawable();
            bg.setShape(GradientDrawable.OVAL);
            int id = tag instanceof Integer ? (Integer) tag : -1;
            bg.setColor(id < 0 ? 0xff2f7fff : DeltamentCardStyle.previewColor(id));
            bg.setStroke(dp(selected ? 3 : 2), selected ? 0xffffffff : 0x33ffffff);
            circle.setBackground(bg);
            cell.setAlpha(selected ? 1f : 0.85f);
        }
    }

    private void applyPreview() {
        if (preview == null) return;
        DeltamentCardStyle.apply(preview, colorId);
    }

    private void save() {
        saveButton.setLoading(true);
        DeltagramCards.savePrefs(currentAccount, colorId, 0, 0.5f, 0.5f, 0.231f, 0, (p, error) -> {
            saveButton.setLoading(false);
            if (error != null) {
                BulletinFactory.of(this).createErrorBulletin(error).show();
                return;
            }
            BulletinFactory.of(this).createSuccessBulletin("Карта сохранена").show();
            finishFragment();
        });
    }
}
