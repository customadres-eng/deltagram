package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;
import static org.telegram.messenger.LocaleController.getString;

import android.content.Context;
import android.graphics.drawable.GradientDrawable;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.util.TypedValue;
import android.view.Gravity;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramCards;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.MessagesController;
import org.telegram.messenger.R;
import org.telegram.messenger.UserObject;
import org.telegram.tgnet.TLRPC;
import org.telegram.ui.ActionBar.BottomSheet;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.BulletinFactory;
import org.telegram.ui.Components.EditTextBoldCursor;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Stories.recorder.ButtonWithCounterView;

/** Compact gift-style transfer sheet for GRAM and Stars. */
public class DeltamentSendSheet extends BottomSheet {

    public DeltamentSendSheet(Context context, int account, String kind, Theme.ResourcesProvider resourcesProvider, Runnable onSent) {
        super(context, false, resourcesProvider);
        fixNavigationBar();
        final boolean ton = "ton".equals(kind);

        LinearLayout content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(22), dp(18), dp(22), dp(22));

        TextView title = new TextView(context);
        title.setText(ton ? "Отправить GRAM" : "Отправить звёзды");
        title.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 22);
        title.setTypeface(AndroidUtilities.bold());
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        title.setGravity(Gravity.CENTER);
        content.addView(title, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView subtitle = new TextView(context);
        subtitle.setText(ton ? "Получатель увидит GRAM на своей TON-карте" : "Получатель увидит звёзды на своей карте Stars");
        subtitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        subtitle.setGravity(Gravity.CENTER);
        content.addView(subtitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 4, 0, 16));

        EditTextBoldCursor username = new EditTextBoldCursor(context);
        username.setHint("@username");
        username.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 18);
        username.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        username.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        username.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        username.setSingleLine(true);
        username.setPadding(dp(14), 0, dp(14), 0);
        username.setBackground(Theme.createRoundRectDrawable(dp(14), Theme.getColor(Theme.key_windowBackgroundGray)));
        content.addView(username, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 52));

        TextView resolved = new TextView(context);
        resolved.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        resolved.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        resolved.setPadding(dp(6), dp(4), 0, 0);
        content.addView(resolved, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        EditTextBoldCursor amount = new EditTextBoldCursor(context);
        amount.setHint(ton ? "Сумма в TON" : "Количество звёзд");
        amount.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 26);
        amount.setTypeface(AndroidUtilities.bold());
        amount.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        amount.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        amount.setInputType(ton ? InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL : InputType.TYPE_CLASS_NUMBER);
        amount.setSingleLine(true);
        amount.setGravity(Gravity.CENTER);
        amount.setPadding(0, 0, 0, 0);
        amount.setBackground(null);
        content.addView(amount, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 58, 0, 14, 0, 0));

        LinearLayout chips = new LinearLayout(context);
        chips.setOrientation(LinearLayout.HORIZONTAL);
        chips.setGravity(Gravity.CENTER);
        long[] values = ton ? new long[]{1, 5, 10, 50} : new long[]{50, 100, 500, 1000};
        for (long value : values) {
            TextView chip = new TextView(context);
            chip.setText((ton ? String.valueOf(value) : String.valueOf(value)) + (ton ? " TON" : " ⭐"));
            chip.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
            chip.setTypeface(AndroidUtilities.bold());
            chip.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
            GradientDrawable bg = new GradientDrawable();
            bg.setCornerRadius(dp(12));
            bg.setColor(Theme.getColor(Theme.key_windowBackgroundGray));
            chip.setBackground(bg);
            chip.setPadding(dp(10), dp(6), dp(10), dp(6));
            chip.setOnClickListener(v -> amount.setText(String.valueOf(value)));
            chips.addView(chip, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 0, 0, dp(6), 0));
        }
        content.addView(chips, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 6, 0, 0));

        ButtonWithCounterView send = new ButtonWithCounterView(context, resourcesProvider).setRound();
        send.setText("Отправить", false);
        send.setOnClickListener(v -> {
            String to = username.getText().toString().trim();
            if (to.startsWith("@")) to = to.substring(1);
            long raw;
            try {
                if (ton) {
                    double t = Double.parseDouble(amount.getText().toString().trim().replace(',', '.'));
                    raw = Math.round(t * 1_000_000_000d);
                    if (raw < 10_000_000L) {
                        BulletinFactory.global().createErrorBulletin("Минимум 0.01 TON").show();
                        return;
                    }
                } else {
                    raw = Long.parseLong(amount.getText().toString().trim());
                    if (raw < 1) {
                        BulletinFactory.global().createErrorBulletin("Введите количество звёзд").show();
                        return;
                    }
                }
            } catch (Exception e) {
                BulletinFactory.global().createErrorBulletin("Введите сумму").show();
                return;
            }
            if (to.isEmpty()) {
                BulletinFactory.global().createErrorBulletin("Введите @username").show();
                return;
            }
            send.setLoading(true);
            final String target = to;
            DeltagramCards.send(account, target, kind, raw, (card, error) -> {
                send.setLoading(false);
                if (error != null) {
                    String message = error;
                    if (message.contains("not found")) message = "Пользователь @" + target + " не найден";
                    else if (message.contains("no_funds") || message.contains("insufficient")) message = "Недостаточно средств";
                    else if (message.contains("yourself")) message = "Нельзя отправить себе";
                    BulletinFactory.global().createErrorBulletin(message).show();
                    return;
                }
                BulletinFactory.global().createSuccessBulletin((ton ? "GRAM отправлен @" : "Звёзды отправлены @") + target).show();
                dismiss();
                if (onSent != null) onSent.run();
            });
        });
        content.addView(send, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 48, 0f, 18, 0, 0));

        username.addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence s, int start, int count, int after) {}
            @Override public void onTextChanged(CharSequence s, int start, int before, int count) {}
            @Override public void afterTextChanged(Editable s) {
                String name = s.toString().trim();
                if (name.startsWith("@")) name = name.substring(1);
                TLRPC.User user = name.isEmpty() ? null : MessagesController.getInstance(account).getUser(name);
                if (user != null) {
                    resolved.setText(UserObject.getUserName(user) + " · @" + name);
                    resolved.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
                } else {
                    resolved.setText("");
                }
            }
        });

        setCustomView(content);
    }
}
