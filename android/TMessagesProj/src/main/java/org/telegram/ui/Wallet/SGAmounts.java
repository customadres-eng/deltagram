package org.telegram.ui.Wallet;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.text.DecimalFormat;
import java.text.DecimalFormatSymbols;
import java.util.Locale;
import org.telegram.messenger.LocaleController;

/** StaticGram amount formatting: GRAM keeps 2/4 decimals, Stars group by commas. */
public final class SGAmounts {
    private static final long COMPACT_FROM = 1000000;

    private SGAmounts() {
    }

    private static String unit(int i) {
        switch (i) {
            case 1: return "K";
            case 2: return "M";
            case 3: return "B";
            case 4: return "T";
            case 5: return "Qa";
            case 6: return "Qi";
            default: return "";
        }
    }

    public static String compact(BigDecimal value) {
        boolean negative = value.signum() < 0;
        BigDecimal abs = value.abs();
        BigDecimal thousand = BigDecimal.valueOf(1000L);
        int i = 0;
        while (abs.compareTo(thousand) >= 0 && i < 6) {
            abs = abs.divide(thousand);
            i++;
        }
        String plain = abs.setScale(i != 0 ? 2 : 0, RoundingMode.DOWN).stripTrailingZeros().toPlainString();
        if (i == 0) {
            return (negative ? "-" : "") + plain;
        }
        String language = LocaleController.getInstance().getCurrentLocale() != null
                ? LocaleController.getInstance().getCurrentLocale().getLanguage() : "en";
        if (!"en".equals(language) && !"ja".equals(language) && !"zh".equals(language) && !"ko".equals(language)) {
            plain = plain.replace('.', ',');
        }
        return (negative ? "-" : "") + plain + " " + unit(i);
    }

    public static String stars(long value, char group) {
        if (Math.abs(value) < COMPACT_FROM) {
            DecimalFormatSymbols symbols = new DecimalFormatSymbols(Locale.US);
            symbols.setGroupingSeparator(group);
            return new DecimalFormat("#,##0", symbols).format(value);
        }
        return compact(BigDecimal.valueOf(value));
    }

    public static String gram(long nano) {
        BigDecimal value = BigDecimal.valueOf(nano, 9);
        if (value.abs().compareTo(BigDecimal.valueOf(COMPACT_FROM)) >= 0) {
            return compact(value);
        }
        int scale = (value.signum() == 0 || value.abs().compareTo(new BigDecimal("0.01")) >= 0) ? 2 : 4;
        RoundingMode mode = RoundingMode.DOWN;
        DecimalFormat format = new DecimalFormat(scale == 2 ? "#,##0.##" : "#,##0.####", new DecimalFormatSymbols(Locale.US));
        format.setRoundingMode(mode);
        return format.format(value.setScale(scale, mode));
    }
}
