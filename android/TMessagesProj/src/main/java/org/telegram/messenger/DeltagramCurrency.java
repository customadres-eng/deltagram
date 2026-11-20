package org.telegram.messenger;

import java.util.Locale;

/**
 * Fiat approximation for the DeltaGram wallet cards. The card itself always
 * stores TON/stars; this helper only converts the shown "≈ ..." line into the
 * currency picked on the Money screen.
 */
public final class DeltagramCurrency {

    public static final String[] CODES = {"RUB", "UAH", "USD", "EUR", "KZT"};
    public static final String[] NAMES = {"Российский рубль", "Украинская гривна", "Доллар США", "Евро", "Казахстанский тенге"};
    public static final String[] SYMBOLS = {"₽", "₴", "$", "€", "₸"};

    private static final double TON_USD = 5.0;
    private static final double STAR_USD = 0.02;
    private static final double[] RATES = {90.0, 41.0, 1.0, 0.92, 450.0};

    private DeltagramCurrency() {
    }

    public static String defaultCode() {
        String country = Locale.getDefault().getCountry();
        if ("UA".equals(country)) return "UAH";
        if ("KZ".equals(country)) return "KZT";
        if ("US".equals(country)) return "USD";
        if ("DE".equals(country) || "FR".equals(country) || "ES".equals(country) || "IT".equals(country)
                || "NL".equals(country) || "PL".equals(country) || "PT".equals(country)) return "EUR";
        return "RUB";
    }

    public static String code() {
        String code = SharedConfig.cardCurrency;
        return code == null || code.isEmpty() ? "RUB" : code;
    }

    public static void setCode(String code) {
        if (code == null) return;
        for (String known : CODES) {
            if (known.equals(code)) {
                SharedConfig.cardCurrency = code;
                MessagesController.getGlobalMainSettings().edit()
                        .putString("deltagram_card_currency", code).apply();
                return;
            }
        }
    }

    public static String symbol() {
        String code = code();
        for (int i = 0; i < CODES.length; i++) {
            if (CODES[i].equals(code)) return SYMBOLS[i];
        }
        return SYMBOLS[0];
    }

    public static String name() {
        String code = code();
        for (int i = 0; i < CODES.length; i++) {
            if (CODES[i].equals(code)) return NAMES[i];
        }
        return NAMES[0];
    }

    private static double rate() {
        String code = code();
        for (int i = 0; i < CODES.length; i++) {
            if (CODES[i].equals(code)) return RATES[i];
        }
        return RATES[0];
    }

    public static String fromTon(long nanoton) {
        return format(nanoton / 1_000_000_000.0 * TON_USD);
    }

    public static String fromStars(long stars) {
        return format(stars * STAR_USD);
    }

    private static String format(double usd) {
        double value = usd * rate();
        String s = String.format(Locale.US, "%,.2f", value);
        return s.replace(',', ' ').replace('.', ',') + " " + symbol();
    }
}
