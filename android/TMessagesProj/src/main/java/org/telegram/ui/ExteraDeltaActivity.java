package org.telegram.ui;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Typeface;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.SeekBar;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.MessagesController;
import org.telegram.messenger.NotificationCenter;
import org.telegram.messenger.R;
import org.telegram.messenger.SharedConfig;
import org.telegram.messenger.UserConfig;
import org.telegram.ui.ActionBar.ActionBar;
import org.telegram.ui.ActionBar.BaseFragment;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.Switch;

/**
 * The Extera Static settings that DeltaGram actually implements: keeping
 * deleted messages, bubble geometry, and the profile privacy extras.
 */
public class ExteraDeltaActivity extends BaseFragment {

    private final int currentAccount = UserConfig.selectedAccount;

    @Override
    public View createView(Context context) {
        actionBar.setBackButtonImage(R.drawable.ic_ab_back);
        actionBar.setAllowOverlayTitle(true);
        actionBar.setTitle("ExteraDelta");
        actionBar.setActionBarMenuOnItemClick(new ActionBar.ActionBarMenuOnItemClick() {
            @Override
            public void onItemClick(int id) {
                if (id == -1) {
                    finishFragment();
                }
            }
        });

        ScrollView scroll = new ScrollView(context);
        scroll.setFillViewport(true);
        scroll.setVerticalScrollBarEnabled(false);
        fragmentView = scroll;

        LinearLayout content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(0, 0, 0, dp(32));
        scroll.addView(content, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        LinearLayout header = new LinearLayout(context);
        header.setOrientation(LinearLayout.HORIZONTAL);
        header.setGravity(Gravity.CENTER_VERTICAL);
        header.setPadding(dp(21), dp(18), dp(21), dp(4));

        android.widget.ImageView logo = new android.widget.ImageView(context);
        logo.setImageResource(R.drawable.deltament_logo);
        logo.setScaleType(android.widget.ImageView.ScaleType.FIT_CENTER);
        header.addView(logo, LayoutHelper.createLinear(56, 56));

        LinearLayout headerTexts = new LinearLayout(context);
        headerTexts.setOrientation(LinearLayout.VERTICAL);
        headerTexts.setPadding(dp(14), 0, 0, 0);
        TextView headerTitle = new TextView(context);
        headerTitle.setText("ExteraDelta");
        headerTitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        headerTitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 21);
        headerTitle.setTypeface(AndroidUtilities.bold());
        headerTexts.addView(headerTitle, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        TextView headerSubtitle = new TextView(context);
        headerSubtitle.setText("DeltaGram edition");
        headerSubtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        headerSubtitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        headerTexts.addView(headerSubtitle, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 0, 2, 0, 0));
        header.addView(headerTexts, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        content.addView(header, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView intro = new TextView(context);
        intro.setText("Настройки ExteraDelta в стиле StaticGram/ExteraGram. Всё работает в этом клиенте и не требует перезапуска, кроме радиуса пузырей — новый размер применится при следующем открытии чата.");
        intro.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        intro.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        intro.setLineSpacing(dp(2), 1f);
        intro.setPadding(dp(21), dp(14), dp(21), dp(14));
        content.addView(intro);

        content.addView(section(context, "Сообщения"));
        addSwitch(content, "Сохранять удалённые сообщения",
                "Сообщения, удалённые собеседником, остаются в чате и в локальной базе",
                SharedConfig.keepDeletedMessages, (view, checked) -> {
                    SharedConfig.keepDeletedMessages = checked;
                    prefs().edit().putBoolean("deltagram_keep_deleted", checked).apply();
                });

        content.addView(section(context, "Внешний вид"));
        addBubbleRadius(content);

        content.addView(section(context, "Персонализация"));
        addSwitch(content, "Скрывать номер телефона",
                "В профиле вместо номера будет написано «Скрыто»",
                SharedConfig.hidePhoneNumber, (view, checked) -> {
                    SharedConfig.hidePhoneNumber = checked;
                    prefs().edit().putBoolean("deltagram_hide_phone", checked).apply();
                });
        addSwitch(content, "Показывать ID и DC",
                "Добавляет строку ID и номер дата-центра в профиль",
                SharedConfig.showIdAndDc, (view, checked) -> {
                    SharedConfig.showIdAndDc = checked;
                    prefs().edit().putBoolean("deltagram_show_id", checked).apply();
                });

        TextView footer = new TextView(context);
        footer.setText("ExteraDelta · DeltaGram");
        footer.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText3));
        footer.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        footer.setGravity(Gravity.CENTER);
        footer.setPadding(0, dp(24), 0, dp(8));
        content.addView(footer);

        return fragmentView;
    }

    private SharedPreferences prefs() {
        return MessagesController.getGlobalMainSettings();
    }

    private TextView section(Context context, String text) {
        TextView view = new TextView(context);
        view.setText(text.toUpperCase());
        view.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        view.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        view.setTypeface(AndroidUtilities.bold());
        view.setLetterSpacing(0.06f);
        view.setPadding(dp(21), dp(22), dp(21), dp(8));
        return view;
    }

    private void addSwitch(LinearLayout parent, String title, String subtitle, boolean checked, Switch.OnCheckedChangeListener listener) {
        Context context = parent.getContext();
        FrameLayout row = new FrameLayout(context);
        row.setBackground(Theme.getSelectorDrawable(false));
        row.setPadding(dp(21), dp(10), dp(21), dp(10));

        LinearLayout texts = new LinearLayout(context);
        texts.setOrientation(LinearLayout.VERTICAL);

        TextView name = new TextView(context);
        name.setText(title);
        name.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        name.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
        texts.addView(name, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView description = new TextView(context);
        description.setText(subtitle);
        description.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        description.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        description.setLineSpacing(dp(1), 1f);
        texts.addView(description, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 3, 0, 0));

        row.addView(texts, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT,
                Gravity.LEFT | Gravity.CENTER_VERTICAL, 0, 0, dp(64), 0));

        Switch toggle = new Switch(context, resourceProvider);
        toggle.setChecked(checked, false);
        toggle.setOnCheckedChangeListener(listener);
        row.addView(toggle, LayoutHelper.createFrame(44, 26, Gravity.RIGHT | Gravity.CENTER_VERTICAL));
        row.setOnClickListener(v -> toggle.setChecked(!toggle.isChecked(), true));
        parent.addView(row, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
    }

    private void addBubbleRadius(LinearLayout parent) {
        Context context = parent.getContext();
        LinearLayout block = new LinearLayout(context);
        block.setOrientation(LinearLayout.VERTICAL);
        block.setPadding(dp(21), dp(10), dp(21), dp(14));

        LinearLayout header = new LinearLayout(context);
        header.setOrientation(LinearLayout.HORIZONTAL);
        header.setGravity(Gravity.CENTER_VERTICAL);

        TextView name = new TextView(context);
        name.setText("Радиус пузырей");
        name.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        name.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
        header.addView(name, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));

        TextView value = new TextView(context);
        value.setText(SharedConfig.bubbleRadius + " dp");
        value.setTextColor(Theme.getColor(Theme.key_featuredStickers_addButton));
        value.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 15);
        value.setTypeface(Typeface.DEFAULT_BOLD);
        header.addView(value, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        block.addView(header, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView description = new TextView(context);
        description.setText("Скругление пузырей сообщений. 0 — прямоугольные, 17 — как в Telegram.");
        description.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        description.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        description.setLineSpacing(dp(1), 1f);
        block.addView(description, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 3, 0, 0));

        SeekBar seek = new SeekBar(context);
        seek.setMax(20);
        seek.setProgress(Math.max(0, Math.min(20, SharedConfig.bubbleRadius)));
        seek.setOnSeekBarChangeListener(new SeekBar.OnSeekBarChangeListener() {
            @Override
            public void onProgressChanged(SeekBar seekBar, int progress, boolean fromUser) {
                value.setText(progress + " dp");
                if (fromUser) {
                    setBubbleRadius(progress);
                }
            }

            @Override
            public void onStartTrackingTouch(SeekBar seekBar) {
            }

            @Override
            public void onStopTrackingTouch(SeekBar seekBar) {
                setBubbleRadius(seekBar.getProgress());
                applyBubbleRadiusToOpenChat();
            }
        });
        block.addView(seek, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 8, 0, 0));
        parent.addView(block, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
    }

    private void setBubbleRadius(int radius) {
        SharedConfig.bubbleRadius = Math.max(0, Math.min(20, radius));
        prefs().edit().putInt("deltagram_bubble_radius", SharedConfig.bubbleRadius).apply();
    }

    private void applyBubbleRadiusToOpenChat() {
        try {
            NotificationCenter.getInstance(currentAccount).postNotificationName(
                    NotificationCenter.updateInterfaces, MessagesController.UPDATE_MASK_ALL);
        } catch (Exception ignored) {
        }
    }
}
