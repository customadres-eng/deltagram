package org.telegram.messenger;

import android.os.Handler;
import android.os.Looper;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/** Public badge lookup as a fallback for stale MTProto profile caches. */
public final class DeltagramVerifications {
    private static final Handler handler = new Handler(Looper.getMainLooper());
    private static final Map<Long, Long> icons = new HashMap<>();
    private static volatile boolean loading;
    private static volatile long lastLoaded;

    private DeltagramVerifications() {}

    public static long iconFor(long userId) {
        if (userId <= 0) return 0;
        if (!loading && System.currentTimeMillis() - lastLoaded > 60000) refresh();
        synchronized (icons) {
            Long icon = icons.get(userId);
            return icon == null ? 0 : icon;
        }
    }

    private static void refresh() {
        loading = true;
        new Thread(() -> {
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL("https://dltagram.duckdns.org/api/verifications/registry").openConnection();
                connection.setConnectTimeout(10000);
                connection.setReadTimeout(10000);
                if (connection.getResponseCode() != HttpURLConnection.HTTP_OK) return;
                StringBuilder body = new StringBuilder();
                try (BufferedReader reader = new BufferedReader(new InputStreamReader(connection.getInputStream(), StandardCharsets.UTF_8))) {
                    String line;
                    while ((line = reader.readLine()) != null) body.append(line);
                }
                JSONObject list = new JSONObject(body.toString()).optJSONObject("icons");
                Map<Long, Long> next = new HashMap<>();
                if (list != null) {
                    java.util.Iterator<String> keys = list.keys();
                    while (keys.hasNext()) {
                        String key = keys.next();
                        try {
                            long id = Long.parseLong(key);
                            long icon = list.optLong(key);
                            if (icon > 0) next.put(id, icon);
                        } catch (NumberFormatException ignored) {}
                    }
                }
                synchronized (icons) {
                    icons.clear();
                    icons.putAll(next);
                    lastLoaded = System.currentTimeMillis();
                }
                handler.post(() -> {
                    for (int account = 0; account < UserConfig.MAX_ACCOUNT_COUNT; account++) {
                        NotificationCenter.getInstance(account).postNotificationName(NotificationCenter.updateInterfaces,
                                MessagesController.UPDATE_MASK_NAME);
                    }
                });
            } catch (Exception ignored) {
                // The native server-projected badge still works while offline.
            } finally {
                if (connection != null) connection.disconnect();
                loading = false;
            }
        }, "DeltaGramVerifications").start();
    }
}
