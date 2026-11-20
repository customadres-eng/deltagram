package org.telegram.messenger;

import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Client for the DeltaGram .ton registry hosted by DeltaMent. The registry is
 * off-chain: names live next to the TON card balance and the owner settles the
 * real TON DNS name by hand, exactly like TON payouts.
 */
public final class DeltagramDomains {

    public static final String BASE_URL = "https://dltagram.duckdns.org";
    private static final String USER_AGENT = "DeltaGramAndroid";
    private static final int TIMEOUT_MS = 15000;

    public interface Callback<T> {
        void onResult(T value, String error);
    }

    public static class Domain {
        public String name;
        public long ownerId;
        public String ownerName;
        public String ownerUsername;
        public long acquiredAt;
        public long lastPrice;
        public boolean listed;
        public long ask;

        public String display() {
            return name + ".ton";
        }
    }

    public static class Check {
        public String name;
        public String state;
        public long price;
        public long basePrice;
        public long ownerId;
        public String ownerName;
        public String ownerUsername;

        public boolean available() {
            return "available".equals(state);
        }

        public boolean yours() {
            return "yours".equals(state);
        }

        public boolean forSale() {
            return "for_sale".equals(state);
        }

        public boolean taken() {
            return "taken".equals(state);
        }
    }

    public static class ListingPage {
        public final List<Domain> items = new ArrayList<>();
        public int total;
        public int page;
        public int limit;
        public boolean hasMore() {
            return items.size() + page * limit < total;
        }
    }

    private static final ExecutorService executor = Executors.newFixedThreadPool(2);
    private static final Handler handler = new Handler(Looper.getMainLooper());

    private DeltagramDomains() {
    }

    public static String normalize(String raw) {
        if (raw == null) {
            return null;
        }
        String name = raw.trim().toLowerCase();
        if (name.startsWith("@")) {
            name = name.substring(1);
        }
        if (name.endsWith(".ton")) {
            name = name.substring(0, name.length() - 4);
        }
        if (name.isEmpty() || name.length() > 32) {
            return null;
        }
        for (int i = 0; i < name.length(); i++) {
            char c = name.charAt(i);
            if ((c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_') {
                return null;
            }
        }
        return name;
    }

    public static String formatTON(long nanoton) {
        double ton = nanoton / 1_000_000_000.0;
        if (ton >= 100) {
            return String.format(java.util.Locale.US, "%.0f", ton);
        } else if (ton >= 10) {
            return String.format(java.util.Locale.US, "%.1f", ton);
        }
        return String.format(java.util.Locale.US, "%.2f", ton);
    }

    public static void check(int account, String name, Callback<Check> callback) {
        String normalized = normalize(name);
        if (normalized == null) {
            callback.onResult(null, "invalid");
            return;
        }
        get(account, "/api/domains/check?name=" + encode(normalized), json -> {
            Check check = new Check();
            check.name = json.optString("name", normalized);
            check.state = json.optString("state");
            check.price = json.optLong("price_nanoton");
            check.basePrice = json.optLong("base_price_nanoton");
            check.ownerId = json.optLong("owner_id");
            check.ownerName = json.optString("owner_name");
            check.ownerUsername = json.optString("owner_username");
            return check;
        }, callback);
    }

    public static void my(int account, final Callback<List<Domain>> callback) {
        get(account, "/api/domains/my", json -> {
            List<Domain> out = new ArrayList<>();
            JSONArray arr = json.optJSONArray("domains");
            if (arr != null) {
                for (int i = 0; i < arr.length(); i++) {
                    out.add(parseDomain(arr.optJSONObject(i)));
                }
            }
            return out;
        }, callback);
    }

    public static void listings(int account, String query, String sort, int page, int limit, Callback<ListingPage> callback) {
        StringBuilder url = new StringBuilder("/api/domains/listings?sort=").append(encode(sort == null ? "date" : sort))
                .append("&page=").append(page).append("&limit=").append(limit);
        if (!TextUtils.isEmpty(query)) {
            url.append("&query=").append(encode(query));
        }
        get(account, url.toString(), json -> {
            ListingPage out = new ListingPage();
            out.total = json.optInt("total");
            out.page = json.optInt("page");
            out.limit = json.optInt("limit", limit);
            JSONArray arr = json.optJSONArray("listings");
            if (arr != null) {
                for (int i = 0; i < arr.length(); i++) {
                    out.items.add(parseDomain(arr.optJSONObject(i)));
                }
            }
            return out;
        }, callback);
    }

    public static void buy(int account, String name, Callback<Domain> callback) {
        JSONObject body = new JSONObject();
        try {
            body.put("name", normalize(name));
        } catch (Exception ignored) {
        }
        post(account, "/api/domains/buy", body, json -> parseDomain(json.optJSONObject("domain")), callback);
    }

    public static void list(int account, String name, long priceNanoton, Callback<Domain> callback) {
        JSONObject body = new JSONObject();
        try {
            body.put("name", normalize(name));
            body.put("price_nanoton", priceNanoton);
        } catch (Exception ignored) {
        }
        post(account, "/api/domains/list", body, json -> parseDomain(json.optJSONObject("domain")), callback);
    }

    public static void unlist(int account, String name, Callback<Domain> callback) {
        JSONObject body = new JSONObject();
        try {
            body.put("name", normalize(name));
        } catch (Exception ignored) {
        }
        post(account, "/api/domains/unlist", body, json -> parseDomain(json.optJSONObject("domain")), callback);
    }

    private static Domain parseDomain(JSONObject json) {
        Domain d = new Domain();
        if (json == null) {
            return d;
        }
        d.name = json.optString("name");
        d.ownerId = json.optLong("owner_id");
        d.ownerName = json.optString("owner_name");
        d.ownerUsername = json.optString("owner_username");
        d.acquiredAt = json.optLong("acquired_at");
        d.lastPrice = json.optLong("last_price_nanoton");
        d.listed = json.optBoolean("listed");
        d.ask = json.optLong("ask_nanoton");
        return d;
    }

    private interface Parser<T> {
        T parse(JSONObject json) throws Exception;
    }

    private interface HttpCallback {
        void onResult(JSONObject json, String error);
    }

    private static void get(int account, String path, Parser<?> parser, Callback<?> callback) {
        request(account, "GET", path, null, parser, callback);
    }

    private static void post(int account, String path, JSONObject body, Parser<?> parser, Callback<?> callback) {
        request(account, "POST", path, body, parser, callback);
    }

    @SuppressWarnings({"unchecked", "rawtypes"})
    private static void request(int account, String method, String path, JSONObject body, Parser parser, Callback callback) {
        final byte[] bodyBytes = body == null ? null : body.toString().getBytes(StandardCharsets.UTF_8);
        final String authorization = DeltagramNativeAuth.authorization(account, method, path, bodyBytes);
        executor.execute(() -> {
            JSONObject result = null;
            String error = null;
            HttpURLConnection conn = null;
            try {
                URL url = new URL(BASE_URL + path);
                conn = (HttpURLConnection) url.openConnection();
                conn.setRequestMethod(method);
                conn.setConnectTimeout(TIMEOUT_MS);
                conn.setReadTimeout(TIMEOUT_MS);
                conn.setRequestProperty("User-Agent", USER_AGENT);
                conn.setRequestProperty("Accept", "application/json");
                if (authorization == null) {
                    throw new IllegalStateException("DeltaGram authentication unavailable");
                }
                conn.setRequestProperty("Authorization", authorization);
                if (bodyBytes != null) {
                    conn.setDoOutput(true);
                    conn.setRequestProperty("Content-Type", "application/json; charset=utf-8");
                    conn.setFixedLengthStreamingMode(bodyBytes.length);
                    try (OutputStream out = conn.getOutputStream()) {
                        out.write(bodyBytes);
                    }
                }
                int status = conn.getResponseCode();
                InputStream stream = status >= 400 ? conn.getErrorStream() : conn.getInputStream();
                String text = read(stream);
                JSONObject json = TextUtils.isEmpty(text) ? new JSONObject() : new JSONObject(text);
                if (status >= 400) {
                    error = json.optString("message", json.optString("error", "HTTP " + status));
                } else {
                    result = json;
                }
            } catch (Exception e) {
                FileLog.e(e);
                error = e.getMessage() == null ? "network" : e.getMessage();
            } finally {
                if (conn != null) {
                    conn.disconnect();
                }
            }
            final JSONObject json = result;
            final String err = error;
            handler.post(() -> {
                if (err != null) {
                    callback.onResult(null, err);
                    return;
                }
                try {
                    callback.onResult(parser.parse(json), null);
                } catch (Exception e) {
                    FileLog.e(e);
                    callback.onResult(null, e.getMessage());
                }
            });
        });
    }

    private static String read(InputStream stream) throws Exception {
        if (stream == null) {
            return "";
        }
        StringBuilder sb = new StringBuilder();
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(stream, StandardCharsets.UTF_8))) {
            String line;
            while ((line = reader.readLine()) != null) {
                sb.append(line);
            }
        }
        return sb.toString();
    }

    private static String encode(String value) {
        try {
            return URLEncoder.encode(value, "UTF-8");
        } catch (Exception e) {
            return value;
        }
    }
}
