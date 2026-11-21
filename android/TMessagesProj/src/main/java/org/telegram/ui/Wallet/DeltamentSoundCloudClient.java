package org.telegram.ui.Wallet;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONArray;
import org.json.JSONObject;
import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.ApplicationLoader;
import org.telegram.messenger.FileLog;
import org.telegram.messenger.Utilities;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Minimal SoundCloud client for the DeltaGram music sheet.
 *
 * SoundCloud has no public developer keys any more, so the web player's
 * public client_id is resolved from soundcloud.com (and cached). Every call is
 * executed on a background thread and delivered back on the UI thread.
 */
public class DeltamentSoundCloudClient {

    public static final String ERROR_CLIENT_ID = "не удалось получить client_id SoundCloud";
    public static final String ERROR_NETWORK = "не удалось загрузить SoundCloud";
    public static final String ERROR_STREAM = "трек недоступен для прослушивания";

    private static final String PREFS = "deltament_soundcloud";
    private static final String KEY_CLIENT_ID = "client_id";
    private static final String UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            + "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";

    private static final Pattern ASSET_PATTERN =
            Pattern.compile("https://a-v2\\.sndcdn\\.com/assets/[^\"'<>\\s]+\\.js");
    private static final Pattern CLIENT_ID_PATTERN =
            Pattern.compile("client_id\\s*[=:]\\s*[\"']?([A-Za-z0-9]{20,48})");

    public interface Callback<T> {
        void onSuccess(T result);
        void onError(String message);
    }

    public static class Track {
        public long id;
        public String title = "";
        public String artist = "";
        public String artworkUrl;
        public String permalink;
        public long durationMs;
        public boolean album;
        public int trackCount;

        public String subtitle() {
            if (album) {
                if (trackCount > 0) {
                    return artist + " · " + trackCount + " треков";
                }
                return artist;
            }
            return artist;
        }
    }

    private static String cachedClientId;

    private DeltamentSoundCloudClient() {
    }

    private static SharedPreferences prefs() {
        final Context context = ApplicationLoader.applicationContext;
        if (context == null) {
            return null;
        }
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    private static void runAsync(Runnable runnable) {
        Utilities.globalQueue.postRunnable(runnable);
    }

    private static <T> void postResult(Callback<T> callback, T value, String error) {
        AndroidUtilities.runOnUIThread(() -> {
            if (error != null) {
                callback.onError(error);
            } else {
                callback.onSuccess(value);
            }
        });
    }

    // ---------------------------------------------------------------- client id

    private static synchronized String getClientIdBlocking() {
        if (cachedClientId != null) {
            return cachedClientId;
        }
        final SharedPreferences prefs = prefs();
        final String saved = prefs == null ? null : prefs.getString(KEY_CLIENT_ID, null);
        if (saved != null && validateClientId(saved)) {
            cachedClientId = saved;
            return saved;
        }
        final String scraped = scrapeClientId();
        if (scraped != null && validateClientId(scraped)) {
            cachedClientId = scraped;
            if (prefs != null) {
                prefs.edit().putString(KEY_CLIENT_ID, scraped).apply();
            }
            return scraped;
        }
        if (scraped != null) {
            // The key was found but the probe failed (rate limit, region).
            // Still worth trying it for real requests.
            cachedClientId = scraped;
            if (prefs != null) {
                prefs.edit().putString(KEY_CLIENT_ID, scraped).apply();
            }
            return scraped;
        }
        return null;
    }

    private static boolean validateClientId(String clientId) {
        try {
            final String body = request("https://api-v2.soundcloud.com/search/tracks?q=test&limit=1&client_id="
                    + URLEncoder.encode(clientId, "UTF-8"), "application/json", true);
            return body != null && body.contains("collection");
        } catch (Throwable e) {
            return false;
        }
    }

    private static String scrapeClientId() {
        try {
            final String html = request("https://soundcloud.com/", "text/html", true);
            if (html == null) {
                return null;
            }
            final Set<String> assets = new LinkedHashSet<>();
            final Matcher matcher = ASSET_PATTERN.matcher(html);
            while (matcher.find() && assets.size() < 24) {
                assets.add(matcher.group());
            }
            for (String asset : assets) {
                try {
                    final String js = request(asset, "application/javascript", false);
                    if (js == null) {
                        continue;
                    }
                    final Matcher idMatcher = CLIENT_ID_PATTERN.matcher(js);
                    if (idMatcher.find()) {
                        return idMatcher.group(1);
                    }
                } catch (Throwable ignored) {
                }
            }
        } catch (Throwable e) {
            FileLog.e(e);
        }
        return null;
    }

    private static String requireClientId() {
        final String clientId = getClientIdBlocking();
        if (clientId == null) {
            throw new IllegalStateException(ERROR_CLIENT_ID);
        }
        return clientId;
    }

    // ------------------------------------------------------------------- http

    private static String request(String url, String accept, boolean failOnError) throws Exception {
        HttpURLConnection connection = null;
        try {
            connection = (HttpURLConnection) new URL(url).openConnection();
            connection.setConnectTimeout(20000);
            connection.setReadTimeout(25000);
            connection.setRequestProperty("User-Agent", UA);
            connection.setRequestProperty("Accept", accept);
            connection.setRequestProperty("Accept-Language", "en-US,en;q=0.9");
            final int code = connection.getResponseCode();
            if (code != HttpURLConnection.HTTP_OK) {
                if (failOnError) {
                    throw new IllegalStateException("HTTP " + code);
                }
                return null;
            }
            try (InputStream input = connection.getInputStream();
                 BufferedReader reader = new BufferedReader(new InputStreamReader(input, "UTF-8"))) {
                final StringBuilder builder = new StringBuilder();
                String line;
                while ((line = reader.readLine()) != null) {
                    builder.append(line);
                }
                return builder.toString();
            }
        } finally {
            if (connection != null) {
                connection.disconnect();
            }
        }
    }

    // ---------------------------------------------------------------- requests

    /** Resolves (and caches) the public client_id without making a real search. */
    public static void preload(Callback<Boolean> callback) {
        runAsync(() -> {
            try {
                requireClientId();
                postResult(callback, Boolean.TRUE, null);
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    public static void searchTracks(String query, Callback<ArrayList<Track>> callback) {
        runAsync(() -> {
            try {
                final String clientId = requireClientId();
                final String url = "https://api-v2.soundcloud.com/search/tracks?q="
                        + URLEncoder.encode(query == null ? "" : query, "UTF-8")
                        + "&limit=40&app_locale=en&client_id=" + URLEncoder.encode(clientId, "UTF-8");
                final String body = request(url, "application/json", true);
                final JSONObject root = new JSONObject(body);
                final JSONArray collection = root.optJSONArray("collection");
                final ArrayList<Track> tracks = new ArrayList<>();
                if (collection != null) {
                    for (int i = 0; i < collection.length(); i++) {
                        final JSONObject item = collection.optJSONObject(i);
                        final Track track = parseTrack(item);
                        if (track != null) {
                            tracks.add(track);
                        }
                    }
                }
                postResult(callback, tracks, null);
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    public static void searchAlbums(String query, Callback<ArrayList<Track>> callback) {
        runAsync(() -> {
            try {
                final String clientId = requireClientId();
                final String url = "https://api-v2.soundcloud.com/search/albums?q="
                        + URLEncoder.encode(query == null ? "" : query, "UTF-8")
                        + "&limit=40&app_locale=en&client_id=" + URLEncoder.encode(clientId, "UTF-8");
                final String body = request(url, "application/json", true);
                final JSONObject root = new JSONObject(body);
                final JSONArray collection = root.optJSONArray("collection");
                final ArrayList<Track> albums = new ArrayList<>();
                if (collection != null) {
                    for (int i = 0; i < collection.length(); i++) {
                        final Track album = parseAlbum(collection.optJSONObject(i));
                        if (album != null) {
                            albums.add(album);
                        }
                    }
                }
                postResult(callback, albums, null);
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    public static void charts(Callback<ArrayList<Track>> callback) {
        runAsync(() -> {
            try {
                final String clientId = requireClientId();
                final String url = "https://api-v2.soundcloud.com/charts?kind=trending&genre=soundcloud:genres:all-music"
                        + "&limit=40&app_locale=en&client_id=" + URLEncoder.encode(clientId, "UTF-8");
                final String body = request(url, "application/json", true);
                final JSONObject root = new JSONObject(body);
                final JSONArray collection = root.optJSONArray("collection");
                final ArrayList<Track> tracks = new ArrayList<>();
                if (collection != null) {
                    for (int i = 0; i < collection.length(); i++) {
                        final JSONObject item = collection.optJSONObject(i);
                        if (item == null) {
                            continue;
                        }
                        final JSONObject trackObject = item.optJSONObject("track");
                        final Track track = parseTrack(trackObject != null ? trackObject : item);
                        if (track != null) {
                            tracks.add(track);
                        }
                    }
                }
                postResult(callback, tracks, null);
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    public static void albumTracks(long albumId, Callback<ArrayList<Track>> callback) {
        runAsync(() -> {
            try {
                final String clientId = requireClientId();
                final String url = "https://api-v2.soundcloud.com/albums/" + albumId + "/tracks?limit=60&client_id="
                        + URLEncoder.encode(clientId, "UTF-8");
                final String body = request(url, "application/json", true);
                final JSONObject root = new JSONObject(body);
                final JSONArray collection = root.optJSONArray("collection");
                final ArrayList<Track> tracks = new ArrayList<>();
                if (collection != null) {
                    for (int i = 0; i < collection.length(); i++) {
                        final Track track = parseTrack(collection.optJSONObject(i));
                        if (track != null) {
                            tracks.add(track);
                        }
                    }
                }
                postResult(callback, tracks, null);
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    public static void resolveStreamUrl(Track track, Callback<String> callback) {
        runAsync(() -> {
            try {
                final String clientId = requireClientId();
                final String trackUrl = "https://api-v2.soundcloud.com/tracks/" + track.id
                        + "?client_id=" + URLEncoder.encode(clientId, "UTF-8");
                final JSONObject root = new JSONObject(request(trackUrl, "application/json", true));
                final String authorization = root.optString("track_authorization", "");
                final JSONObject media = root.optJSONObject("media");
                final JSONArray transcodings = media == null ? null : media.optJSONArray("transcodings");
                if (transcodings == null || transcodings.length() == 0) {
                    postResult(callback, null, ERROR_STREAM);
                    return;
                }
                String transcodingUrl = null;
                String fallbackUrl = null;
                for (int i = 0; i < transcodings.length(); i++) {
                    final JSONObject item = transcodings.optJSONObject(i);
                    if (item == null) {
                        continue;
                    }
                    final String url = item.optString("url", "");
                    if (url.isEmpty()) {
                        continue;
                    }
                    final JSONObject format = item.optJSONObject("format");
                    final String protocol = format == null ? "" : format.optString("protocol", "");
                    final String preset = item.optString("preset", "");
                    if ("progressive".equals(protocol)) {
                        transcodingUrl = url;
                        break;
                    }
                    if (fallbackUrl == null || "aac_160k".equals(preset)) {
                        fallbackUrl = url;
                    }
                }
                if (transcodingUrl == null) {
                    transcodingUrl = fallbackUrl;
                }
                if (transcodingUrl == null) {
                    postResult(callback, null, ERROR_STREAM);
                    return;
                }
                final StringBuilder requestUrl = new StringBuilder(transcodingUrl);
                requestUrl.append(transcodingUrl.contains("?") ? '&' : '?');
                requestUrl.append("client_id=").append(URLEncoder.encode(clientId, "UTF-8"));
                if (!authorization.isEmpty()) {
                    requestUrl.append("&track_authorization=").append(URLEncoder.encode(authorization, "UTF-8"));
                }
                final JSONObject stream = new JSONObject(request(requestUrl.toString(), "application/json", true));
                final String url = stream.optString("url", "");
                if (url.isEmpty()) {
                    postResult(callback, null, ERROR_STREAM);
                } else {
                    postResult(callback, url, null);
                }
            } catch (Throwable e) {
                postResult(callback, null, mapError(e));
            }
        });
    }

    // ------------------------------------------------------------------ parsing

    private static Track parseTrack(JSONObject object) {
        if (object == null) {
            return null;
        }
        final Track track = new Track();
        track.id = object.optLong("id");
        if (track.id == 0) {
            return null;
        }
        track.title = object.optString("title", "");
        final JSONObject user = object.optJSONObject("user");
        if (user != null) {
            track.artist = user.optString("username", "");
        }
        track.durationMs = object.optLong("duration");
        track.permalink = object.optString("permalink_url", "");
        String artwork = object.optString("artwork_url", "");
        if (artwork == null || artwork.isEmpty() || "null".equals(artwork)) {
            artwork = user != null ? user.optString("avatar_url", "") : "";
        }
        if (!artwork.isEmpty() && !"null".equals(artwork)) {
            track.artworkUrl = artwork;
        }
        return track;
    }

    private static Track parseAlbum(JSONObject object) {
        if (object == null) {
            return null;
        }
        final Track album = new Track();
        album.album = true;
        album.id = object.optLong("id");
        if (album.id == 0) {
            return null;
        }
        album.title = object.optString("title", "");
        final JSONObject user = object.optJSONObject("user");
        if (user != null) {
            album.artist = user.optString("username", "");
        }
        album.trackCount = object.optInt("track_count");
        album.permalink = object.optString("permalink_url", "");
        String artwork = object.optString("artwork_url", "");
        if (artwork != null && !artwork.isEmpty() && !"null".equals(artwork)) {
            album.artworkUrl = artwork;
        }
        return album;
    }

    private static String mapError(Throwable error) {
        if (error instanceof IllegalStateException && ERROR_CLIENT_ID.equals(error.getMessage())) {
            return ERROR_CLIENT_ID;
        }
        final String message = error == null ? null : error.getMessage();
        if (message != null && message.contains("HTTP 401")) {
            cachedClientId = null;
        }
        return ERROR_NETWORK;
    }
}
