package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.animation.ArgbEvaluator;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.graphics.Canvas;
import android.graphics.LinearGradient;
import android.graphics.Paint;
import android.graphics.Shader;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.text.Editable;
import android.text.InputFilter;
import android.text.InputType;
import android.text.Spanned;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.TextWatcher;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.inputmethod.EditorInfo;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.DeltagramDomains;
import org.telegram.messenger.LocaleController;
import org.telegram.messenger.UserConfig;
import org.telegram.ui.ActionBar.ActionBar;
import org.telegram.ui.ActionBar.AlertDialog;
import org.telegram.ui.ActionBar.BaseFragment;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.BulletinFactory;
import org.telegram.ui.Components.CubicBezierInterpolator;
import org.telegram.ui.Components.EditTextBoldCursor;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Stories.recorder.ButtonWithCounterView;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Native .ton registry screen, laid out like StaticGram's SGDomainsActivity but
 * backed by the DeltaGram server: search, buy, sell, unlist and the market list
 * all use {@link DeltagramDomains}.
 */
public class DeltamentDomainsActivity extends BaseFragment {

    private static final long CHECK_DELAY_MS = 350;
    private static final String[] SORTS = {"date", "price_asc", "price_desc", "name"};
    private static final String[] SORT_LABELS = {"Новые", "Дешевле", "Дороже", "По имени"};

    private final int currentAccount = UserConfig.selectedAccount;

    private ScrollView scrollView;
    private LinearLayout content;
    private DomainHeroView heroView;
    private EditTextBoldCursor searchEdit;
    private ImageView clearButton;
    private TextView stateChip;
    private ButtonWithCounterView buyButton;
    private LinearLayout myList;
    private TextView myEmpty;
    private LinearLayout listingsList;
    private TextView listingsEmpty;
    private TextView listingsMore;
    private final TextView[] sortChips = new TextView[4];

    private final List<DeltagramDomains.Domain> listings = new ArrayList<>();
    private int listingsTotal;
    private int listingsPage;
    private boolean listingsLoading;
    private boolean myLoading;
    private int sortIndex;

    private DeltagramDomains.Check lastCheck;
    private int checkSeq;
    private boolean destroyed;

    private final Runnable checkRunnable = this::runCheck;

    @Override
    public View createView(Context context) {
        actionBar.setBackButtonImage(org.telegram.messenger.R.drawable.ic_ab_back);
        actionBar.setAllowOverlayTitle(true);
        actionBar.setTitle("Домены .ton");
        actionBar.setActionBarMenuOnItemClick(new ActionBar.ActionBarMenuOnItemClick() {
            @Override
            public void onItemClick(int id) {
                if (id == -1) {
                    finishFragment();
                }
            }
        });

        fragmentView = new FrameLayout(context);
        fragmentView.setBackgroundColor(Theme.getColor(Theme.key_windowBackgroundWhite));

        scrollView = new ScrollView(context);
        scrollView.setVerticalScrollBarEnabled(false);
        scrollView.setClipToPadding(false);
        scrollView.setPadding(0, 0, 0, dp(28));
        ((FrameLayout) fragmentView).addView(scrollView, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));

        content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(16), dp(4), dp(16), dp(24));
        scrollView.addView(content, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        heroView = new DomainHeroView(context);
        content.addView(heroView, LayoutHelper.createLinear(124, 124, Gravity.CENTER_HORIZONTAL, 0, 8, 0, 0));
        heroView.setDomain("имя.ton", 0xffffd45e);

        TextView intro = new TextView(context);
        intro.setText("Домен .ton — короткое имя для кошелька и профиля. Имя закрепляется за аккаунтом, его можно продать или оставить себе.");
        intro.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        intro.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        intro.setGravity(Gravity.CENTER);
        intro.setLineSpacing(dp(2), 1f);
        content.addView(intro, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 16, 14, 16, 18));

        content.addView(createSearchRow(context), LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        stateChip = new TextView(context);
        stateChip.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        stateChip.setTypeface(AndroidUtilities.bold());
        stateChip.setGravity(Gravity.CENTER);
        stateChip.setPadding(dp(12), 0, dp(12), 0);
        stateChip.setVisibility(View.GONE);
        content.addView(stateChip, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, 30, Gravity.CENTER_HORIZONTAL, 16, 10, 16, 0));

        buyButton = new ButtonWithCounterView(context, resourceProvider).setRound();
        buyButton.setVisibility(View.GONE);
        buyButton.setOnClickListener(v -> onBuyClicked());
        content.addView(buyButton, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 48, 0f, 14, 0, 0));

        TextView myTitle = sectionTitle(context, "Мои домены");
        content.addView(myTitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 16, 28, 16, 8));

        myList = new LinearLayout(context);
        myList.setOrientation(LinearLayout.VERTICAL);
        myList.setBackground(Theme.createRoundRectDrawable(dp(16), Theme.getColor(Theme.key_windowBackgroundGray)));
        content.addView(myList, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        myEmpty = new TextView(context);
        myEmpty.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        myEmpty.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        myEmpty.setGravity(Gravity.CENTER);
        myEmpty.setPadding(dp(12), dp(18), dp(12), dp(18));
        myEmpty.setText("У вас пока нет доменов.\nНайдите свободное имя выше и купите его.");
        myList.addView(myEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView listingsTitle = sectionTitle(context, "На продаже");
        content.addView(listingsTitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 16, 28, 16, 8));
        content.addView(createSortRow(context), LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        listingsList = new LinearLayout(context);
        listingsList.setOrientation(LinearLayout.VERTICAL);
        listingsList.setBackground(Theme.createRoundRectDrawable(dp(16), Theme.getColor(Theme.key_windowBackgroundGray)));
        content.addView(listingsList, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        listingsEmpty = new TextView(context);
        listingsEmpty.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        listingsEmpty.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        listingsEmpty.setGravity(Gravity.CENTER);
        listingsEmpty.setPadding(dp(12), dp(18), dp(12), dp(18));
        listingsList.addView(listingsEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        listingsMore = new TextView(context);
        listingsMore.setText("Показать ещё");
        listingsMore.setTextColor(Theme.getColor(Theme.key_featuredStickers_addButton));
        listingsMore.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 15);
        listingsMore.setTypeface(AndroidUtilities.bold());
        listingsMore.setGravity(Gravity.CENTER);
        listingsMore.setPadding(dp(12), dp(14), dp(12), dp(14));
        listingsMore.setOnClickListener(v -> loadListings(true));
        listingsMore.setVisibility(View.GONE);
        content.addView(listingsMore, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView note = new TextView(context);
        note.setText("Комиссия при продаже — 10%. Баланс списывается с TON-карты. Само имя владелец закрепляет в TON DNS вручную, как и обычный вывод TON.");
        note.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        note.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 12);
        note.setGravity(Gravity.CENTER);
        note.setLineSpacing(dp(2), 1f);
        content.addView(note, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 12, 24, 12, 0));

        refreshMy();
        refreshListings();
        return fragmentView;
    }

    private TextView sectionTitle(Context context, String text) {
        TextView view = new TextView(context);
        view.setText(text);
        view.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        view.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 14);
        view.setTypeface(AndroidUtilities.bold());
        return view;
    }

    private View createSearchRow(Context context) {
        LinearLayout row = new LinearLayout(context);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), 0, dp(8), 0);
        row.setBackground(Theme.createRoundRectDrawable(dp(16), Theme.getColor(Theme.key_windowBackgroundGray)));

        ImageView icon = new ImageView(context);
        icon.setImageResource(org.telegram.messenger.R.drawable.outline_search_1_24);
        icon.setColorFilter(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        row.addView(icon, LayoutHelper.createLinear(24, 24, 16, 0, 0, 10, 0));

        searchEdit = new EditTextBoldCursor(context);
        searchEdit.setBackground(null);
        searchEdit.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 18);
        searchEdit.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        searchEdit.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        searchEdit.setHint("Введите имя");
        searchEdit.setCursorColor(Theme.getColor(Theme.key_featuredStickers_addButton));
        searchEdit.setCursorSize(dp(20));
        searchEdit.setCursorWidth(1.5f);
        searchEdit.setSingleLine(true);
        searchEdit.setPadding(0, dp(16), 0, dp(16));
        searchEdit.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS | InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD);
        searchEdit.setImeOptions(EditorInfo.IME_ACTION_DONE);
        searchEdit.setFilters(new InputFilter[]{(source, start, end, dest, dstart, dend) -> {
            StringBuilder sb = new StringBuilder();
            for (int i = start; i < end; i++) {
                char c = source.charAt(i);
                if (c >= 'A' && c <= 'Z') {
                    c = (char) (c + 32);
                }
                if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
                    sb.append(c);
                }
            }
            return sb.toString();
        }});
        searchEdit.addTextChangedListener(new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence s, int start, int count, int after) {
            }

            @Override
            public void onTextChanged(CharSequence s, int start, int before, int count) {
            }

            @Override
            public void afterTextChanged(Editable s) {
                onSearchChanged();
            }
        });
        searchEdit.setOnEditorActionListener((v, actionId, event) -> {
            if (actionId == EditorInfo.IME_ACTION_DONE || (event != null && event.getKeyCode() == KeyEvent.KEYCODE_ENTER)) {
                AndroidUtilities.hideKeyboard(searchEdit);
                runCheck();
                return true;
            }
            return false;
        });
        row.addView(searchEdit, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f, 16, 0, 4, 0));

        TextView suffix = new TextView(context);
        suffix.setText(".ton");
        suffix.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        suffix.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 18);
        suffix.setTypeface(AndroidUtilities.bold());
        row.addView(suffix, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, 16, 4, 0, 4, 0));

        clearButton = new ImageView(context);
        clearButton.setImageResource(org.telegram.messenger.R.drawable.msg_clear);
        clearButton.setScaleType(ImageView.ScaleType.CENTER);
        clearButton.setColorFilter(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        clearButton.setBackground(Theme.createSelectorDrawable(Theme.getColor(Theme.key_listSelector), 1));
        clearButton.setVisibility(View.GONE);
        clearButton.setOnClickListener(v -> {
            searchEdit.setText("");
            lastCheck = null;
            updateChip(null);
        });
        row.addView(clearButton, LayoutHelper.createLinear(36, 36, 16));
        return row;
    }

    private View createSortRow(Context context) {
        HorizontalScrollView scroll = new HorizontalScrollView(context);
        scroll.setHorizontalScrollBarEnabled(false);
        scroll.setClipToPadding(false);
        LinearLayout row = new LinearLayout(context);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setPadding(0, 0, 0, dp(10));
        for (int i = 0; i < SORTS.length; i++) {
            final int index = i;
            TextView chip = new TextView(context);
            chip.setText(SORT_LABELS[i]);
            chip.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
            chip.setTypeface(AndroidUtilities.bold());
            chip.setGravity(Gravity.CENTER);
            chip.setPadding(dp(12), 0, dp(12), 0);
            chip.setOnClickListener(v -> {
                sortIndex = index;
                updateSortChips();
                refreshListings();
            });
            row.addView(chip, LayoutHelper.createLinear(LayoutHelper.WRAP_CONTENT, 30, 0f, 0, 0, 6, 0));
            sortChips[i] = chip;
        }
        scroll.addView(row);
        updateSortChips();
        return scroll;
    }

    private void updateSortChips() {
        for (int i = 0; i < sortChips.length; i++) {
            TextView chip = sortChips[i];
            if (chip == null) {
                continue;
            }
            boolean selected = i == sortIndex;
            chip.setTextColor(selected ? Theme.getColor(Theme.key_windowBackgroundWhiteBlackText) : Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
            chip.setBackground(selected
                    ? Theme.createRoundRectDrawable(dp(15), Theme.getColor(Theme.key_listSelector))
                    : Theme.createRoundRectDrawable(dp(15), 0x00000000));
        }
    }

    private void onSearchChanged() {
        String name = DeltagramDomains.normalize(searchEdit.getText().toString());
        clearButton.setVisibility(TextUtils.isEmpty(searchEdit.getText()) ? View.GONE : View.VISIBLE);
        if (name == null || name.isEmpty()) {
            heroView.setDomain("имя.ton", 0xffffd45e);
            lastCheck = null;
            updateChip(null);
            AndroidUtilities.cancelRunOnUIThread(checkRunnable);
            return;
        }
        heroView.setDomain(name + ".ton", 0xffffd45e);
        AndroidUtilities.cancelRunOnUIThread(checkRunnable);
        setChipText("Проверяем…", 0xff9aa0a6, 0x149aa0a6);
        buyButton.setVisibility(View.GONE);
        AndroidUtilities.runOnUIThread(checkRunnable, CHECK_DELAY_MS);
    }

    private void runCheck() {
        final String name = DeltagramDomains.normalize(searchEdit.getText().toString());
        if (name == null || name.isEmpty()) {
            return;
        }
        final int seq = ++checkSeq;
        DeltagramDomains.check(currentAccount, name, (check, error) -> {
            if (destroyed || seq != checkSeq) {
                return;
            }
            if (error != null || check == null) {
                setChipText("Не удалось проверить", 0xffe05252, 0x1ae05252);
                buyButton.setVisibility(View.GONE);
                return;
            }
            lastCheck = check;
            updateChip(check);
        });
    }

    private void updateChip(DeltagramDomains.Check check) {
        if (check == null) {
            stateChip.setVisibility(View.GONE);
            if (buyButton != null) {
                buyButton.setVisibility(View.GONE);
            }
            heroView.setAccent(0xffffd45e);
            return;
        }
        stateChip.setVisibility(View.VISIBLE);
        switch (check.state) {
            case "available":
                setChipText("Свободен · " + DeltagramDomains.formatTON(check.price) + " TON", 0xff4caf7d, 0x1a4caf7d);
                buyButton.setText("Купить за " + DeltagramDomains.formatTON(check.price) + " TON", false);
                buyButton.setVisibility(View.VISIBLE);
                heroView.setAccent(0xff4caf7d);
                break;
            case "for_sale":
                setChipText("Продаётся · " + DeltagramDomains.formatTON(check.price) + " TON", 0xffffb648, 0x1affb648);
                buyButton.setText("Купить за " + DeltagramDomains.formatTON(check.price) + " TON", false);
                buyButton.setVisibility(View.VISIBLE);
                heroView.setAccent(0xffffb648);
                break;
            case "yours":
                setChipText("Ваш домен", 0xff5aa7ff, 0x1a5aa7ff);
                buyButton.setVisibility(View.GONE);
                heroView.setAccent(0xff5aa7ff);
                break;
            case "taken": {
                String owner = !TextUtils.isEmpty(check.ownerUsername) ? "@" + check.ownerUsername
                        : !TextUtils.isEmpty(check.ownerName) ? check.ownerName : "другим пользователем";
                setChipText("Занят · " + owner, 0xffe05252, 0x1ae05252);
                buyButton.setVisibility(View.GONE);
                heroView.setAccent(0xffe05252);
                break;
            }
            default:
                setChipText("Не удалось проверить", 0xffe05252, 0x1ae05252);
                buyButton.setVisibility(View.GONE);
        }
    }

    private void setChipText(String text, int textColor, int backgroundColor) {
        stateChip.setText(text);
        stateChip.setTextColor(textColor);
        GradientDrawable bg = new GradientDrawable();
        bg.setCornerRadius(dp(15));
        bg.setColor(backgroundColor);
        stateChip.setBackground(bg);
    }

    private void onBuyClicked() {
        if (lastCheck == null) {
            return;
        }
        final DeltagramDomains.Check check = lastCheck;
        if (!check.available() && !check.forSale()) {
            return;
        }
        new AlertDialog.Builder(getParentActivity())
                .setTitle("Купить " + check.name + ".ton?")
                .setMessage("С карты спишется " + DeltagramDomains.formatTON(check.price) + " TON.\nДомен сразу закрепится за вашим аккаунтом.")
                .setPositiveButton("Купить", (dialog, which) -> doBuy(check.name))
                .setNegativeButton("Отмена", null)
                .show();
    }

    private void doBuy(String name) {
        buyButton.setLoading(true);
        DeltagramDomains.buy(currentAccount, name, (domain, error) -> {
            buyButton.setLoading(false);
            if (error != null) {
                showError(error);
                return;
            }
            BulletinFactory.of(this).createSuccessBulletin("Домен " + name + ".ton куплен").show();
            searchEdit.setText("");
            lastCheck = null;
            updateChip(null);
            refreshMy();
            refreshListings();
        });
    }

    private void refreshMy() {
        myLoading = true;
        renderMy(new ArrayList<>());
        DeltagramDomains.my(currentAccount, (domains, error) -> {
            if (destroyed) {
                return;
            }
            myLoading = false;
            renderMy(domains == null ? new ArrayList<>() : domains);
        });
    }

    private void renderMy(List<DeltagramDomains.Domain> domains) {
        myList.removeAllViews();
        if (domains.isEmpty()) {
            myEmpty.setVisibility(View.VISIBLE);
            myList.addView(myEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
            return;
        }
        myEmpty.setVisibility(View.GONE);
        for (DeltagramDomains.Domain domain : domains) {
            myList.addView(createRow(getContext(), domain, true, false), LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 64));
        }
    }

    private void refreshListings() {
        listings.clear();
        listingsPage = 0;
        listingsTotal = 0;
        loadListings(false);
    }

    private void loadListings(boolean append) {
        if (listingsLoading) {
            return;
        }
        listingsLoading = true;
        if (!append) {
            listingsListingsLoading();
        }
        final int page = append ? listingsPage + 1 : 0;
        DeltagramDomains.listings(currentAccount, "", SORTS[sortIndex], page, 30, (result, error) -> {
            if (destroyed) {
                return;
            }
            listingsLoading = false;
            if (result == null) {
                if (!append) {
                    listingsList.removeAllViews();
                    listingsEmpty.setText("Не удалось загрузить список");
                    listingsEmpty.setVisibility(View.VISIBLE);
                    listingsList.addView(listingsEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
                }
                listingsMore.setVisibility(View.GONE);
                return;
            }
            listingsPage = result.page;
            listingsTotal = result.total;
            if (!append) {
                listings.clear();
            }
            listings.addAll(result.items);
            renderListings();
        });
    }

    private void listingsListingsLoading() {
        listingsList.removeAllViews();
        listingsEmpty.setText("Загрузка…");
        listingsEmpty.setVisibility(View.VISIBLE);
        listingsList.addView(listingsEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
        listingsMore.setVisibility(View.GONE);
    }

    private void renderListings() {
        listingsList.removeAllViews();
        if (listings.isEmpty()) {
            listingsEmpty.setText("Пока никто не продаёт домены");
            listingsEmpty.setVisibility(View.VISIBLE);
            listingsList.addView(listingsEmpty, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
            listingsMore.setVisibility(View.GONE);
            return;
        }
        listingsEmpty.setVisibility(View.GONE);
        for (DeltagramDomains.Domain domain : listings) {
            listingsList.addView(createRow(getContext(), domain, false, false), LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 64));
        }
        listingsMore.setVisibility(listings.size() < listingsTotal ? View.VISIBLE : View.GONE);
    }

    private View createRow(Context context, DeltagramDomains.Domain domain, boolean mine, boolean divider) {
        FrameLayout row = new FrameLayout(context) {
            @Override
            protected void dispatchDraw(Canvas canvas) {
                super.dispatchDraw(canvas);
                if (divider) {
                    canvas.drawRect(dp(72), getHeight() - Math.max(1, dp(0.5f)), getWidth(), getHeight(), Theme.dividerPaint);
                }
            }
        };
        row.setWillNotDraw(false);
        row.setBackground(Theme.getSelectorDrawable(false));
        row.setOnClickListener(v -> {
            if (mine) {
                openManageSheet(domain);
            } else {
                openBuySheet(domain);
            }
        });

        TextView avatar = new TextView(context);
        avatar.setText(domain.name == null || domain.name.isEmpty() ? "?" : domain.name.substring(0, 1).toUpperCase(Locale.ROOT));
        avatar.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 20);
        avatar.setTypeface(AndroidUtilities.bold());
        avatar.setTextColor(0xffffffff);
        avatar.setGravity(Gravity.CENTER);
        GradientDrawable bg = new GradientDrawable(GradientDrawable.Orientation.TL_BR,
                mine ? new int[]{0xff8e2424, 0xff3a151a} : new int[]{0xffe0a020, 0xff8a5a12});
        bg.setCornerRadius(dp(12));
        avatar.setBackground(bg);
        row.addView(avatar, LayoutHelper.createFrame(44, 44, Gravity.CENTER_VERTICAL | Gravity.LEFT, 14, 0, 0, 0));

        LinearLayout texts = new LinearLayout(context);
        texts.setOrientation(LinearLayout.VERTICAL);
        TextView title = new TextView(context);
        title.setText(domain.display());
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        title.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 16);
        title.setTypeface(AndroidUtilities.bold());
        title.setSingleLine(true);
        title.setEllipsize(TextUtils.TruncateAt.END);
        texts.addView(title);

        TextView subtitle = new TextView(context);
        subtitle.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 13);
        subtitle.setSingleLine(true);
        subtitle.setEllipsize(TextUtils.TruncateAt.END);
        if (mine) {
            if (domain.listed) {
                subtitle.setText("На продаже · " + DeltagramDomains.formatTON(domain.ask) + " TON");
                subtitle.setTextColor(0xffffb648);
            } else {
                subtitle.setText("Ваш домен");
                subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
            }
        } else {
            String seller = !TextUtils.isEmpty(domain.ownerUsername) ? "@" + domain.ownerUsername
                    : !TextUtils.isEmpty(domain.ownerName) ? domain.ownerName : "DeltaGram";
            subtitle.setText(seller);
            subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        }
        texts.addView(subtitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 2, 0, 0));
        row.addView(texts, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.CENTER_VERTICAL | Gravity.LEFT, 72, 0, 44, 0));

        TextView price = new TextView(context);
        price.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 15);
        price.setTypeface(AndroidUtilities.bold());
        price.setGravity(Gravity.RIGHT | Gravity.CENTER_VERTICAL);
        if (!mine) {
            price.setText(DeltagramDomains.formatTON(domain.ask) + " TON");
            price.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        } else {
            price.setText("›");
            price.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 24);
            price.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        }
        row.addView(price, LayoutHelper.createFrame(LayoutHelper.WRAP_CONTENT, LayoutHelper.MATCH_PARENT, Gravity.RIGHT | Gravity.CENTER_VERTICAL, 0, 0, 16, 0));
        return row;
    }

    private void openBuySheet(DeltagramDomains.Domain domain) {
        new AlertDialog.Builder(getParentActivity())
                .setTitle(domain.display())
                .setMessage("Цена: " + DeltagramDomains.formatTON(domain.ask) + " TON\nПродавец: " + sellerOf(domain))
                .setPositiveButton("Купить", (dialog, which) -> doBuy(domain.name))
                .setNegativeButton("Отмена", null)
                .show();
    }

    private String sellerOf(DeltagramDomains.Domain domain) {
        if (!TextUtils.isEmpty(domain.ownerUsername)) {
            return "@" + domain.ownerUsername;
        }
        if (!TextUtils.isEmpty(domain.ownerName)) {
            return domain.ownerName;
        }
        return "DeltaGram";
    }

    private void openManageSheet(DeltagramDomains.Domain domain) {
        final boolean listed = domain.listed;
        final List<String> items = new ArrayList<>();
        items.add(listed ? "Снять с продажи" : "Продать");
        if (listed) {
            items.add("Изменить цену");
        }
        items.add("Скопировать имя");
        new AlertDialog.Builder(getParentActivity())
                .setTitle(domain.display())
                .setItems(items.toArray(new CharSequence[0]), (dialog, which) -> {
                    String action = items.get(which);
                    if ("Снять с продажи".equals(action)) {
                        doUnlist(domain.name);
                    } else if ("Изменить цену".equals(action)) {
                        showPriceDialog(domain);
                    } else if ("Продать".equals(action)) {
                        showPriceDialog(domain);
                    } else {
                        copy(domain.display());
                    }
                })
                .setNegativeButton("Закрыть", null)
                .show();
    }

    private void showPriceDialog(DeltagramDomains.Domain domain) {
        final Context context = getParentActivity();
        EditTextBoldCursor edit = new EditTextBoldCursor(context);
        edit.setTextSize(TypedValue.COMPLEX_UNIT_DIP, 18);
        edit.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        edit.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
        edit.setHint("Например 10");
        edit.setCursorColor(Theme.getColor(Theme.key_featuredStickers_addButton));
        edit.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
        edit.setSingleLine(true);
        FrameLayout box = new FrameLayout(context);
        box.setPadding(dp(24), dp(8), dp(24), 0);
        box.addView(edit, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, 44));
        if (domain.ask > 0) {
            edit.setText(DeltagramDomains.formatTON(domain.ask));
            edit.setSelection(edit.length());
        }
        new AlertDialog.Builder(context)
                .setTitle("Цена в TON")
                .setView(box)
                .setPositiveButton("Выставить", (dialog, which) -> {
                    String raw = edit.getText().toString().trim().replace(',', '.');
                    double ton;
                    try {
                        ton = Double.parseDouble(raw);
                    } catch (Exception e) {
                        BulletinFactory.of(this).createErrorBulletin("Введите цену").show();
                        return;
                    }
                    long nanoton = Math.round(ton * 1_000_000_000d);
                    if (nanoton < 1_000_000_000L || nanoton > 1_000_000_000_000_000L) {
                        BulletinFactory.of(this).createErrorBulletin("Цена должна быть от 1 до 1 000 000 TON").show();
                        return;
                    }
                    doList(domain.name, nanoton);
                })
                .setNegativeButton("Отмена", null)
                .show();
    }

    private void doList(String name, long priceNanoton) {
        DeltagramDomains.list(currentAccount, name, priceNanoton, (domain, error) -> {
            if (error != null) {
                showError(error);
                return;
            }
            BulletinFactory.of(this).createSuccessBulletin(name + ".ton выставлен на продажу").show();
            refreshMy();
            refreshListings();
        });
    }

    private void doUnlist(String name) {
        DeltagramDomains.unlist(currentAccount, name, (domain, error) -> {
            if (error != null) {
                showError(error);
                return;
            }
            BulletinFactory.of(this).createSuccessBulletin(name + ".ton снят с продажи").show();
            refreshMy();
            refreshListings();
        });
    }

    private void copy(String text) {
        try {
            ClipboardManager cm = (ClipboardManager) getParentActivity().getSystemService(Context.CLIPBOARD_SERVICE);
            cm.setPrimaryClip(ClipData.newPlainText("domain", text));
            BulletinFactory.of(this).createCopyBulletin("Скопировано: " + text).show();
        } catch (Exception ignored) {
        }
    }

    private void showError(String error) {
        String message = error;
        if (message == null) {
            message = "Ошибка";
        } else if (message.contains("not enough balance") || message.contains("no_funds")) {
            message = "Недостаточно TON на карте";
        } else if (message.contains("already belongs")) {
            message = "Этот домен уже ваш";
        } else if (message.contains("not for sale")) {
            message = "Домен больше не продаётся";
        } else if (message.contains("forbidden")) {
            message = "Нет доступа";
        } else if (message.contains("invalid domain")) {
            message = "Некорректное имя домена";
        }
        BulletinFactory.of(this).createErrorBulletin(message).show();
    }

    @Override
    public void onFragmentDestroy() {
        destroyed = true;
        AndroidUtilities.cancelRunOnUIThread(checkRunnable);
        super.onFragmentDestroy();
    }

    /** Small gradient card with the domain name, the hero of the SG screen. */
    private static class DomainHeroView extends View {
        private final Paint background = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final Paint stroke = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final TextPaint namePaint = new TextPaint(Paint.ANTI_ALIAS_FLAG);
        private final TextPaint suffixPaint = new TextPaint(Paint.ANTI_ALIAS_FLAG);
        private String domain = "имя.ton";
        private int accent = 0xffffd45e;

        DomainHeroView(Context context) {
            super(context);
            namePaint.setTypeface(Typeface.create("sans-serif", Typeface.BOLD));
            namePaint.setTextAlign(Paint.Align.CENTER);
            namePaint.setColor(0xffffffff);
            suffixPaint.setTextAlign(Paint.Align.CENTER);
            suffixPaint.setColor(0x99ffffff);
            suffixPaint.setTypeface(Typeface.create("sans-serif", Typeface.BOLD));
            stroke.setStyle(Paint.Style.STROKE);
            stroke.setStrokeWidth(dp(1.5f));
        }

        void setDomain(String value, int accentColor) {
            domain = value == null ? "" : value;
            accent = accentColor;
            invalidate();
        }

        void setAccent(int color) {
            accent = color;
            invalidate();
        }

        @Override
        protected void onDraw(Canvas canvas) {
            final float w = getWidth();
            final float h = getHeight();
            final float r = dp(18);
            background.setShader(new LinearGradient(0, 0, w, h, 0xff1a1014, blend(0xff2c1218, accent, 0.35f), Shader.TileMode.CLAMP));
            canvas.drawRoundRect(0, 0, w, h, r, r, background);
            stroke.setColor(blend(0xff3a2026, accent, 0.6f));
            canvas.drawRoundRect(dp(0.75f), dp(0.75f), w - dp(0.75f), h - dp(0.75f), r, r, stroke);

            String text = domain;
            String suffix = "";
            if (text.endsWith(".ton")) {
                suffix = ".ton";
                text = text.substring(0, text.length() - 4);
            }
            float size = dp(30);
            namePaint.setTextSize(size);
            while (size > dp(12) && namePaint.measureText(text) > w - dp(20)) {
                size -= dp(1);
                namePaint.setTextSize(size);
            }
            suffixPaint.setTextSize(dp(12));
            float centerY = h / 2f;
            if (suffix.isEmpty()) {
                canvas.drawText(text, w / 2f, centerY + size / 3f, namePaint);
            } else {
                canvas.drawText(text, w / 2f, centerY - dp(2), namePaint);
                canvas.drawText(suffix, w / 2f, centerY + dp(18), suffixPaint);
            }
        }

        private static int blend(int from, int to, float t) {
            return (Integer) new ArgbEvaluator().evaluate(t, from, to);
        }
    }
}
