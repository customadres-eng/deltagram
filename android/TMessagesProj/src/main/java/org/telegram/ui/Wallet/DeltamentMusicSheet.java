package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.InputType;
import android.text.TextUtils;
import android.text.TextWatcher;
import android.util.LruCache;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.View;
import android.view.inputmethod.EditorInfo;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import androidx.media3.common.AudioAttributes;
import androidx.media3.common.C;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.exoplayer.ExoPlayer;

import org.json.JSONArray;
import org.json.JSONObject;
import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.ApplicationLoader;
import org.telegram.messenger.FileLog;
import org.telegram.messenger.MediaController;
import org.telegram.messenger.MessageObject;
import org.telegram.messenger.MessagesController;
import org.telegram.messenger.UserConfig;
import org.telegram.messenger.Utilities;
import org.telegram.tgnet.ConnectionsManager;
import org.telegram.tgnet.TLRPC;
import org.telegram.ui.ActionBar.BottomSheet;
import org.telegram.ui.Adapters.FiltersView;
import org.telegram.ui.Components.LayoutHelper;

import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.ArrayList;
import java.util.Iterator;

/**
 * DeltaGram music sheet. Reproduces the reference layout: close button, centred
 * title, queue action, rounded search field, three segmented switches and a
 * result list. SoundCloud is the default source; "Из чатов" searches music in
 * the user's messages through the regular MTProto global search.
 */
public class DeltamentMusicSheet extends BottomSheet {

    private static final int COLOR_BG = 0xFF1C1C1E;
    private static final int COLOR_FIELD = 0xFF2C2C2E;
    private static final int COLOR_BLUE = 0xFF3B7DF6;
    private static final int COLOR_TEXT = 0xFFFFFFFF;
    private static final int COLOR_TEXT_SECONDARY = 0xFF8E8E93;
    private static final int COLOR_STATUS = 0xFFE8E8EA;

    private static final int SOURCE_SOUNDCLOUD = 0;
    private static final int SOURCE_CHATS = 1;

    private static final int TYPE_TRACKS = 0;
    private static final int TYPE_ALBUMS = 1;

    private static final int MODE_SEARCH = 0;
    private static final int MODE_LIKED = 1;
    private static final int MODE_FOR_YOU = 2;

    private static final String PREFS_LIKES = "deltament_soundcloud_likes";
    private static final String KEY_LIKES = "tracks";

    private static final LruCache<String, Bitmap> artworkCache = new LruCache<String, Bitmap>(12 * 1024 * 1024) {
        @Override
        protected int sizeOf(String key, Bitmap value) {
            return value.getByteCount();
        }
    };

    private final Context context;
    private final int currentAccount;

    private LinearLayout root;
    private TextView titleView;
    private TextView queueButton;
    private EditText searchField;
    private SegmentRow sourceRow;
    private SegmentRow typeRow;
    private SegmentRow modeRow;
    private TextView statusView;
    private LinearLayout listContainer;
    private ScrollView scrollView;
    private LinearLayout miniPlayer;
    private TextView miniControl;
    private TextView miniTitle;
    private View searchContainer;

    private final ArrayList<DeltamentSoundCloudClient.Track> visible = new ArrayList<>();
    private final ArrayList<DeltamentSoundCloudClient.Track> queue = new ArrayList<>();
    private int queueIndex = -1;
    private DeltamentSoundCloudClient.Track currentTrack;

    private ExoPlayer player;
    private boolean prepared;
    private boolean playing;
    private boolean queueMode;

    private int source = SOURCE_SOUNDCLOUD;
    private int type = TYPE_TRACKS;
    private int mode = MODE_SEARCH;
    private int loadGeneration;

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final Runnable searchRunnable = this::reload;

    public DeltamentMusicSheet(Context context) {
        super(context, false);
        this.context = context;
        this.currentAccount = UserConfig.selectedAccount;

        setBackgroundColor(COLOR_BG);
        setApplyTopPadding(false);
        setApplyBottomPadding(true);
        setCanDismissWithSwipe(true);
        setDimBehindAlpha(75);

        final int screenHeight = AndroidUtilities.displaySize.y;
        final int sheetHeight = Math.max(dp(340), Math.min(dp(700), screenHeight - dp(90)));

        root = new FixedHeightLinearLayout(context, sheetHeight);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(16), dp(10), dp(16), dp(6));
        setCustomView(root);

        buildHeader();
        buildSearch();
        buildSegments();
        buildStatus();
        buildList();
        buildMiniPlayer();

        setOnDismissListener(new Runnable() {
            @Override
            public void run() {
                releasePlayer();
                handler.removeCallbacksAndMessages(null);
            }
        });

        DeltamentSoundCloudClient.preload(new DeltamentSoundCloudClient.Callback<Boolean>() {
            @Override
            public void onSuccess(Boolean result) {
                if (source == SOURCE_SOUNDCLOUD && mode == MODE_SEARCH && visible.isEmpty() && TextUtils.isEmpty(query())) {
                    statusView.setText("введите запрос");
                }
            }

            @Override
            public void onError(String message) {
                if (source == SOURCE_SOUNDCLOUD && mode == MODE_SEARCH && visible.isEmpty() && TextUtils.isEmpty(query())) {
                    statusView.setText(message);
                }
            }
        });
    }

    // ---------------------------------------------------------------- ui build

    private void buildHeader() {
        final FrameLayout header = new FrameLayout(context);
        root.addView(header, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 46));

        final TextView close = new TextView(context);
        close.setText("✕");
        close.setTextSize(18);
        close.setTextColor(COLOR_TEXT);
        close.setGravity(Gravity.CENTER);
        close.setBackground(rounded(COLOR_FIELD, dp(18)));
        close.setOnClickListener(v -> dismiss());
        header.addView(close, LayoutHelper.createFrame(36, 36, Gravity.START | Gravity.CENTER_VERTICAL));

        titleView = new TextView(context);
        titleView.setText("Музыка");
        titleView.setTextSize(18);
        titleView.setTextColor(COLOR_TEXT);
        titleView.setTypeface(Typeface.create(Typeface.DEFAULT, Typeface.BOLD));
        titleView.setSingleLine(true);
        titleView.setGravity(Gravity.CENTER);
        header.addView(titleView, LayoutHelper.createFrame(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, Gravity.CENTER));

        queueButton = new TextView(context);
        queueButton.setText("Очередь");
        queueButton.setTextSize(15);
        queueButton.setTextColor(COLOR_BLUE);
        queueButton.setGravity(Gravity.CENTER);
        queueButton.setPadding(dp(10), dp(6), dp(2), dp(6));
        queueButton.setOnClickListener(v -> setQueueMode(!queueMode));
        header.addView(queueButton, LayoutHelper.createFrame(LayoutHelper.WRAP_CONTENT, 36, Gravity.END | Gravity.CENTER_VERTICAL));
    }

    private void buildSearch() {
        final LinearLayout container = new LinearLayout(context);
        searchContainer = container;
        container.setOrientation(LinearLayout.VERTICAL);
        root.addView(container, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 0, 8, 0, 0));

        searchField = new EditText(context);
        searchField.setHint("Поиск");
        searchField.setTextSize(16);
        searchField.setTextColor(COLOR_TEXT);
        searchField.setHintTextColor(COLOR_TEXT_SECONDARY);
        searchField.setSingleLine(true);
        searchField.setInputType(InputType.TYPE_CLASS_TEXT);
        searchField.setImeOptions(EditorInfo.IME_ACTION_SEARCH);
        searchField.setBackground(rounded(COLOR_FIELD, dp(24)));
        searchField.setPadding(dp(20), 0, dp(20), 0);
        searchField.setGravity(Gravity.CENTER_VERTICAL);
        searchField.addTextChangedListener(new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence s, int start, int count, int after) {
            }

            @Override
            public void onTextChanged(CharSequence s, int start, int before, int count) {
            }

            @Override
            public void afterTextChanged(Editable s) {
                handler.removeCallbacks(searchRunnable);
                handler.postDelayed(searchRunnable, 350);
            }
        });
        container.addView(searchField, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 48));
    }

    private void buildSegments() {
        sourceRow = new SegmentRow(context, new String[]{"SoundCloud", "Из чатов"});
        sourceRow.setSelected(source, false);
        sourceRow.setListener(index -> {
            if (source == index) {
                return;
            }
            source = index;
            reload();
        });
        root.addView(sourceRow, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 42, 0, 0, 10, 0, 0));

        typeRow = new SegmentRow(context, new String[]{"Треки", "Альбомы"});
        typeRow.setSelected(type, false);
        typeRow.setListener(index -> {
            if (type == index) {
                return;
            }
            type = index;
            if (mode != MODE_SEARCH) {
                mode = MODE_SEARCH;
                modeRow.setSelected(mode, false);
            }
            reload();
        });
        root.addView(typeRow, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 42, 0, 0, 10, 0, 0));

        modeRow = new SegmentRow(context, new String[]{"Поиск", "Лайкнутые", "Для вас"});
        modeRow.setSelected(mode, false);
        modeRow.setListener(index -> {
            if (mode == index) {
                return;
            }
            mode = index;
            reload();
        });
        root.addView(modeRow, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 42, 0, 0, 10, 0, 0));
    }

    private void buildStatus() {
        statusView = new TextView(context);
        statusView.setTextSize(14);
        statusView.setTextColor(COLOR_STATUS);
        statusView.setPadding(dp(8), dp(12), dp(8), dp(4));
        statusView.setText("");
        root.addView(statusView, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));
    }

    private void buildList() {
        scrollView = new ScrollView(context);
        scrollView.setVerticalScrollBarEnabled(false);
        scrollView.setFillViewport(false);
        listContainer = new LinearLayout(context);
        listContainer.setOrientation(LinearLayout.VERTICAL);
        scrollView.addView(listContainer, LayoutHelper.createScroll(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.TOP));
        root.addView(scrollView, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 0, 1f));
    }

    private void buildMiniPlayer() {
        miniPlayer = new LinearLayout(context);
        miniPlayer.setOrientation(LinearLayout.HORIZONTAL);
        miniPlayer.setGravity(Gravity.CENTER_VERTICAL);
        miniPlayer.setBackground(rounded(COLOR_FIELD, dp(16)));
        miniPlayer.setPadding(dp(6), dp(6), dp(6), dp(6));
        miniPlayer.setVisibility(View.GONE);
        root.addView(miniPlayer, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, 54, 0, 0, 8, 0, 0));

        miniControl = new TextView(context);
        miniControl.setText("▶");
        miniControl.setTextSize(18);
        miniControl.setTextColor(COLOR_TEXT);
        miniControl.setGravity(Gravity.CENTER);
        miniControl.setOnClickListener(v -> togglePlayback());
        miniPlayer.addView(miniControl, LayoutHelper.createLinear(40, 40));

        miniTitle = new TextView(context);
        miniTitle.setTextSize(14);
        miniTitle.setTextColor(COLOR_TEXT);
        miniTitle.setSingleLine(true);
        miniTitle.setEllipsize(TextUtils.TruncateAt.END);
        miniTitle.setPadding(dp(4), 0, dp(4), 0);
        miniPlayer.addView(miniTitle, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));

        final TextView close = new TextView(context);
        close.setText("✕");
        close.setTextSize(14);
        close.setTextColor(COLOR_TEXT_SECONDARY);
        close.setGravity(Gravity.CENTER);
        close.setOnClickListener(v -> stopPlayback());
        miniPlayer.addView(close, LayoutHelper.createLinear(36, 36));
    }

    private static GradientDrawable rounded(int color, float radius) {
        final GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(radius);
        return drawable;
    }

    // --------------------------------------------------------------- behaviour

    private String query() {
        return searchField.getText() == null ? "" : searchField.getText().toString().trim();
    }

    private void reload() {
        if (queueMode) {
            return;
        }
        handler.removeCallbacks(searchRunnable);
        final int generation = ++loadGeneration;
        final String query = query();

        if (source == SOURCE_CHATS) {
            if (TextUtils.isEmpty(query)) {
                setStatus("введите запрос");
                renderTracks(new ArrayList<>());
                return;
            }
            searchChats(query, generation);
            return;
        }

        if (mode == MODE_LIKED) {
            final ArrayList<DeltamentSoundCloudClient.Track> liked = readLiked();
            if (!TextUtils.isEmpty(query)) {
                final String lower = query.toLowerCase();
                for (int i = liked.size() - 1; i >= 0; i--) {
                    final DeltamentSoundCloudClient.Track track = liked.get(i);
                    if (!track.title.toLowerCase().contains(lower) && !track.artist.toLowerCase().contains(lower)) {
                        liked.remove(i);
                    }
                }
            }
            renderTracks(liked);
            setStatus(liked.isEmpty() ? "нет лайкнутых треков" : "");
            return;
        }

        if (mode == MODE_FOR_YOU) {
            setStatus("загрузка…");
            DeltamentSoundCloudClient.charts(new ListCallback(generation));
            return;
        }

        if (TextUtils.isEmpty(query)) {
            setStatus("введите запрос");
            renderTracks(new ArrayList<>());
            return;
        }

        setStatus("загрузка…");
        if (type == TYPE_ALBUMS) {
            DeltamentSoundCloudClient.searchAlbums(query, new ListCallback(generation));
        } else {
            DeltamentSoundCloudClient.searchTracks(query, new ListCallback(generation));
        }
    }

    private class ListCallback implements DeltamentSoundCloudClient.Callback<ArrayList<DeltamentSoundCloudClient.Track>> {
        private final int generation;

        ListCallback(int generation) {
            this.generation = generation;
        }

        @Override
        public void onSuccess(ArrayList<DeltamentSoundCloudClient.Track> result) {
            if (generation != loadGeneration || queueMode) {
                return;
            }
            renderTracks(result);
            setStatus(result.isEmpty() ? "ничего не найдено" : "");
        }

        @Override
        public void onError(String message) {
            if (generation != loadGeneration || queueMode) {
                return;
            }
            renderTracks(new ArrayList<>());
            setStatus(message);
        }
    }

    private void setStatus(String text) {
        statusView.setText(text == null ? "" : text);
    }

    private void renderTracks(ArrayList<DeltamentSoundCloudClient.Track> tracks) {
        visible.clear();
        visible.addAll(tracks);
        listContainer.removeAllViews();
        for (DeltamentSoundCloudClient.Track track : tracks) {
            listContainer.addView(createTrackRow(track));
        }
    }

    private View createTrackRow(DeltamentSoundCloudClient.Track track) {
        final LinearLayout row = new LinearLayout(context);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(8), dp(8), dp(8), dp(8));
        row.setBackground(rounded(0x00000000, dp(12)));
        row.setOnClickListener(v -> {
            if (track.album) {
                openAlbum(track);
            } else {
                playTrack(track);
            }
        });

        final ImageView artwork = new ImageView(context);
        artwork.setScaleType(ImageView.ScaleType.CENTER_CROP);
        artwork.setBackground(rounded(COLOR_FIELD, dp(10)));
        artwork.setClipToOutline(true);
        row.addView(artwork, LayoutHelper.createLinear(46, 46));
        loadArtwork(track.artworkUrl, artwork);

        final LinearLayout textColumn = new LinearLayout(context);
        textColumn.setOrientation(LinearLayout.VERTICAL);
        textColumn.setPadding(dp(12), 0, dp(8), 0);
        row.addView(textColumn, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));

        final TextView title = new TextView(context);
        title.setText(track.title);
        title.setTextSize(15);
        title.setTextColor(COLOR_TEXT);
        title.setSingleLine(true);
        title.setEllipsize(TextUtils.TruncateAt.END);
        textColumn.addView(title, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        final TextView subtitle = new TextView(context);
        subtitle.setText(track.subtitle());
        subtitle.setTextSize(13);
        subtitle.setTextColor(COLOR_TEXT_SECONDARY);
        subtitle.setSingleLine(true);
        subtitle.setEllipsize(TextUtils.TruncateAt.END);
        subtitle.setPadding(0, dp(2), 0, 0);
        textColumn.addView(subtitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        if (track.album) {
            final TextView arrow = new TextView(context);
            arrow.setText("›");
            arrow.setTextSize(22);
            arrow.setTextColor(COLOR_TEXT_SECONDARY);
            arrow.setGravity(Gravity.CENTER);
            row.addView(arrow, LayoutHelper.createLinear(28, 36));
        } else {
            final TextView like = new TextView(context);
            final boolean liked = isLiked(track.id);
            like.setText(liked ? "♥" : "♡");
            like.setTextSize(18);
            like.setTextColor(liked ? COLOR_BLUE : COLOR_TEXT_SECONDARY);
            like.setGravity(Gravity.CENTER);
            like.setOnClickListener(v -> {
                toggleLike(track);
                final boolean nowLiked = isLiked(track.id);
                like.setText(nowLiked ? "♥" : "♡");
                like.setTextColor(nowLiked ? COLOR_BLUE : COLOR_TEXT_SECONDARY);
            });
            row.addView(like, LayoutHelper.createLinear(36, 36));

            final TextView duration = new TextView(context);
            duration.setText(formatDuration(track.durationMs));
            duration.setTextSize(12);
            duration.setTextColor(COLOR_TEXT_SECONDARY);
            duration.setGravity(Gravity.CENTER);
            row.addView(duration, LayoutHelper.createLinear(44, 36));
        }
        return row;
    }

    private static String formatDuration(long durationMs) {
        if (durationMs <= 0) {
            return "";
        }
        final long totalSeconds = durationMs / 1000;
        final long minutes = totalSeconds / 60;
        final long seconds = totalSeconds % 60;
        return minutes + ":" + (seconds < 10 ? "0" : "") + seconds;
    }

    private void openAlbum(DeltamentSoundCloudClient.Track album) {
        final int generation = ++loadGeneration;
        setStatus("загрузка альбома…");
        DeltamentSoundCloudClient.albumTracks(album.id, new DeltamentSoundCloudClient.Callback<ArrayList<DeltamentSoundCloudClient.Track>>() {
            @Override
            public void onSuccess(ArrayList<DeltamentSoundCloudClient.Track> result) {
                if (generation != loadGeneration || queueMode) {
                    return;
                }
                renderTracks(result);
                setStatus(result.isEmpty() ? "альбом пуст" : album.title + " · " + album.artist);
            }

            @Override
            public void onError(String message) {
                if (generation != loadGeneration || queueMode) {
                    return;
                }
                setStatus(message);
            }
        });
    }

    // --------------------------------------------------------------- chat search

    private void searchChats(String query, int generation) {
        setStatus("загрузка…");
        final TLRPC.TL_messages_searchGlobal request = new TLRPC.TL_messages_searchGlobal();
        request.q = query;
        request.limit = 40;
        request.filter = new TLRPC.TL_inputMessagesFilterMusic();
        request.flags |= 1;
        request.folder_id = 0;
        request.offset_id = 0;
        request.offset_rate = 0;
        request.offset_peer = new TLRPC.TL_inputPeerEmpty();

        ConnectionsManager.getInstance(currentAccount).sendRequestTyped(request, AndroidUtilities::runOnUIThread, (res, error) -> {
            if (generation != loadGeneration || queueMode) {
                return;
            }
            if (error != null || res == null) {
                setStatus("не удалось найти музыку в чатах");
                renderTracks(new ArrayList<>());
                return;
            }
            final MessagesController messagesController = MessagesController.getInstance(currentAccount);
            messagesController.putUsers(res.users, false);
            messagesController.putChats(res.chats, false);

            final ArrayList<DeltamentSoundCloudClient.Track> tracks = new ArrayList<>();
            final ArrayList<MessageObject> messageObjects = new ArrayList<>();
            for (int i = 0; i < res.messages.size(); i++) {
                final TLRPC.Message message = res.messages.get(i);
                final MessageObject messageObject = new MessageObject(currentAccount, message, false, true);
                if (!messageObject.isMusic() || messageObject.getDocument() == null) {
                    continue;
                }
                final DeltamentSoundCloudClient.Track track = new DeltamentSoundCloudClient.Track();
                track.id = messageObject.getId();
                track.title = messageObject.getMusicTitle();
                track.artist = messageObject.getMusicAuthor();
                track.durationMs = (long) messageObject.getDuration() * 1000L;
                if (TextUtils.isEmpty(track.title)) {
                    track.title = "Аудио";
                }
                tracks.add(track);
                messageObjects.add(messageObject);
            }
            renderChatTracks(tracks, messageObjects, query, res.next_rate, res.count);
            setStatus(tracks.isEmpty() ? "в чатах ничего не найдено" : "");
        });
    }

    private void renderChatTracks(ArrayList<DeltamentSoundCloudClient.Track> tracks,
                                  ArrayList<MessageObject> messageObjects,
                                  String query, int nextRate, int totalCount) {
        visible.clear();
        visible.addAll(tracks);
        listContainer.removeAllViews();
        for (int i = 0; i < tracks.size(); i++) {
            final DeltamentSoundCloudClient.Track track = tracks.get(i);
            final MessageObject messageObject = messageObjects.get(i);
            final LinearLayout row = new LinearLayout(context);
            row.setOrientation(LinearLayout.HORIZONTAL);
            row.setGravity(Gravity.CENTER_VERTICAL);
            row.setPadding(dp(8), dp(8), dp(8), dp(8));
            row.setOnClickListener(v -> {
                final MediaController.PlaylistGlobalSearchParams params =
                        new MediaController.PlaylistGlobalSearchParams(query, 0, 0, 0,
                                new FiltersView.MediaFilterData(0, 0, new TLRPC.TL_inputMessagesFilterMusic(),
                                        FiltersView.FILTER_TYPE_MUSIC));
                params.endReached = false;
                params.nextSearchRate = nextRate;
                params.totalCount = totalCount;
                params.folderId = 0;
                final ArrayList<MessageObject> playlist = new ArrayList<>(messageObjects);
                MediaController.getInstance().setPlaylist(playlist, messageObject, 0, params);
            });

            final View placeholder = new View(context);
            placeholder.setBackground(rounded(COLOR_FIELD, dp(10)));
            row.addView(placeholder, LayoutHelper.createLinear(46, 46));

            final LinearLayout textColumn = new LinearLayout(context);
            textColumn.setOrientation(LinearLayout.VERTICAL);
            textColumn.setPadding(dp(12), 0, dp(8), 0);
            row.addView(textColumn, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));

            final TextView title = new TextView(context);
            title.setText(track.title);
            title.setTextSize(15);
            title.setTextColor(COLOR_TEXT);
            title.setSingleLine(true);
            title.setEllipsize(TextUtils.TruncateAt.END);
            textColumn.addView(title, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

            final TextView subtitle = new TextView(context);
            subtitle.setText(track.artist);
            subtitle.setTextSize(13);
            subtitle.setTextColor(COLOR_TEXT_SECONDARY);
            subtitle.setSingleLine(true);
            subtitle.setEllipsize(TextUtils.TruncateAt.END);
            textColumn.addView(subtitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

            final TextView duration = new TextView(context);
            duration.setText(formatDuration(track.durationMs));
            duration.setTextSize(12);
            duration.setTextColor(COLOR_TEXT_SECONDARY);
            duration.setGravity(Gravity.CENTER);
            row.addView(duration, LayoutHelper.createLinear(48, 36));

            listContainer.addView(row);
        }
    }

    // ------------------------------------------------------------------ playback

    private void playTrack(DeltamentSoundCloudClient.Track track) {
        if (track == null) {
            return;
        }
        int existing = -1;
        for (int i = 0; i < queue.size(); i++) {
            if (queue.get(i).id == track.id) {
                existing = i;
                break;
            }
        }
        if (existing < 0) {
            queue.add(track);
            existing = queue.size() - 1;
        }
        queueIndex = existing;
        currentTrack = track;
        miniTitle.setText(track.title);
        miniPlayer.setVisibility(View.VISIBLE);
        miniControl.setText("…");
        setStatus("загрузка трека…");

        DeltamentSoundCloudClient.resolveStreamUrl(track, new DeltamentSoundCloudClient.Callback<String>() {
            @Override
            public void onSuccess(String url) {
                startPlayback(url);
            }

            @Override
            public void onError(String message) {
                miniControl.setText("▶");
                setStatus(message);
            }
        });
    }

    private void startPlayback(String url) {
        releasePlayer();
        try {
            player = new ExoPlayer.Builder(context).build();
            player.setAudioAttributes(new AudioAttributes.Builder()
                    .setContentType(C.AUDIO_CONTENT_TYPE_MUSIC)
                    .setUsage(C.USAGE_MEDIA)
                    .build(), true);
            player.addListener(new Player.Listener() {
                @Override
                public void onPlaybackStateChanged(int state) {
                    if (state == Player.STATE_READY) {
                        prepared = true;
                        playing = player.isPlaying();
                        miniControl.setText("⏸");
                        setStatus("");
                    } else if (state == Player.STATE_ENDED) {
                        playNext();
                    }
                }

                @Override
                public void onPlayerError(PlaybackException error) {
                    playing = false;
                    prepared = false;
                    miniControl.setText("▶");
                    setStatus(DeltamentSoundCloudClient.ERROR_STREAM);
                }
            });
            player.setMediaItem(MediaItem.fromUri(url));
            player.prepare();
            player.play();
        } catch (Throwable e) {
            FileLog.e(e);
            releasePlayer();
            miniControl.setText("▶");
            setStatus(DeltamentSoundCloudClient.ERROR_STREAM);
        }
    }

    private void togglePlayback() {
        if (player == null || !prepared) {
            if (currentTrack != null) {
                playTrack(currentTrack);
            }
            return;
        }
        try {
            if (player.isPlaying()) {
                player.pause();
                playing = false;
                miniControl.setText("▶");
            } else {
                player.play();
                playing = true;
                miniControl.setText("⏸");
            }
        } catch (Throwable e) {
            FileLog.e(e);
        }
    }

    private void playNext() {
        if (queueIndex + 1 < queue.size()) {
            queueIndex++;
            playTrack(queue.get(queueIndex));
        } else {
            playing = false;
            prepared = false;
            miniControl.setText("▶");
        }
    }

    private void stopPlayback() {
        releasePlayer();
        miniPlayer.setVisibility(View.GONE);
        currentTrack = null;
        queueIndex = -1;
    }

    private void releasePlayer() {
        if (player != null) {
            try {
                player.release();
            } catch (Throwable ignored) {
            }
            player = null;
        }
        prepared = false;
        playing = false;
    }

    // -------------------------------------------------------------------- queue

    private void setQueueMode(boolean enabled) {
        if (queueMode == enabled) {
            return;
        }
        queueMode = enabled;
        handler.removeCallbacks(searchRunnable);
        if (enabled) {
            titleView.setText("Очередь");
            queueButton.setText("Готово");
            searchContainer.setVisibility(View.GONE);
            sourceRow.setVisibility(View.GONE);
            typeRow.setVisibility(View.GONE);
            modeRow.setVisibility(View.GONE);
            renderQueue();
        } else {
            titleView.setText("Музыка");
            queueButton.setText("Очередь");
            searchContainer.setVisibility(View.VISIBLE);
            sourceRow.setVisibility(View.VISIBLE);
            typeRow.setVisibility(View.VISIBLE);
            modeRow.setVisibility(View.VISIBLE);
            reload();
        }
    }

    private void renderQueue() {
        listContainer.removeAllViews();
        if (queue.isEmpty()) {
            setStatus("очередь пуста");
            return;
        }
        setStatus("");
        for (int i = 0; i < queue.size(); i++) {
            final DeltamentSoundCloudClient.Track track = queue.get(i);
            final LinearLayout row = new LinearLayout(context);
            row.setOrientation(LinearLayout.HORIZONTAL);
            row.setGravity(Gravity.CENTER_VERTICAL);
            row.setPadding(dp(8), dp(10), dp(8), dp(10));
            if (i == queueIndex) {
                row.setBackground(rounded(0xFF2A2A2D, dp(12)));
            }
            row.setOnClickListener(v -> {
                queueIndex = indexOfTrack(track);
                playTrack(track);
                renderQueue();
            });

            final LinearLayout textColumn = new LinearLayout(context);
            textColumn.setOrientation(LinearLayout.VERTICAL);
            row.addView(textColumn, LayoutHelper.createLinear(0, LayoutHelper.WRAP_CONTENT, 1f));

            final TextView title = new TextView(context);
            title.setText(track.title);
            title.setTextSize(15);
            title.setTextColor(i == queueIndex ? COLOR_BLUE : COLOR_TEXT);
            title.setSingleLine(true);
            title.setEllipsize(TextUtils.TruncateAt.END);
            textColumn.addView(title, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

            final TextView subtitle = new TextView(context);
            subtitle.setText(track.artist);
            subtitle.setTextSize(13);
            subtitle.setTextColor(COLOR_TEXT_SECONDARY);
            subtitle.setSingleLine(true);
            subtitle.setEllipsize(TextUtils.TruncateAt.END);
            textColumn.addView(subtitle, LayoutHelper.createLinear(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

            final TextView remove = new TextView(context);
            remove.setText("✕");
            remove.setTextSize(14);
            remove.setTextColor(COLOR_TEXT_SECONDARY);
            remove.setGravity(Gravity.CENTER);
            remove.setOnClickListener(v -> {
                final int index = indexOfTrack(track);
                if (index >= 0) {
                    queue.remove(index);
                    if (index < queueIndex) {
                        queueIndex--;
                    } else if (index == queueIndex) {
                        releasePlayer();
                        currentTrack = null;
                        queueIndex = -1;
                        miniPlayer.setVisibility(View.GONE);
                    }
                }
                renderQueue();
            });
            row.addView(remove, LayoutHelper.createLinear(36, 36));
            listContainer.addView(row);
        }
    }

    private int indexOfTrack(DeltamentSoundCloudClient.Track track) {
        for (int i = 0; i < queue.size(); i++) {
            if (queue.get(i).id == track.id) {
                return i;
            }
        }
        return -1;
    }

    // -------------------------------------------------------------------- likes

    private SharedPreferences prefs() {
        final Context appContext = ApplicationLoader.applicationContext;
        if (appContext == null) {
            return null;
        }
        return appContext.getSharedPreferences(PREFS_LIKES, Context.MODE_PRIVATE);
    }

    private ArrayList<DeltamentSoundCloudClient.Track> readLiked() {
        final ArrayList<DeltamentSoundCloudClient.Track> tracks = new ArrayList<>();
        final SharedPreferences preferences = prefs();
        if (preferences == null) {
            return tracks;
        }
        final String raw = preferences.getString(KEY_LIKES, null);
        if (raw == null) {
            return tracks;
        }
        try {
            final JSONArray array = new JSONArray(raw);
            for (int i = 0; i < array.length(); i++) {
                final JSONObject object = array.optJSONObject(i);
                if (object == null) {
                    continue;
                }
                final DeltamentSoundCloudClient.Track track = new DeltamentSoundCloudClient.Track();
                track.id = object.optLong("id");
                track.title = object.optString("title", "");
                track.artist = object.optString("artist", "");
                track.artworkUrl = object.optString("artwork", null);
                track.durationMs = object.optLong("duration");
                tracks.add(track);
            }
        } catch (Throwable e) {
            FileLog.e(e);
        }
        return tracks;
    }

    private boolean isLiked(long id) {
        for (DeltamentSoundCloudClient.Track track : readLiked()) {
            if (track.id == id) {
                return true;
            }
        }
        return false;
    }

    private void toggleLike(DeltamentSoundCloudClient.Track track) {
        final ArrayList<DeltamentSoundCloudClient.Track> liked = readLiked();
        boolean removed = false;
        for (Iterator<DeltamentSoundCloudClient.Track> iterator = liked.iterator(); iterator.hasNext(); ) {
            if (iterator.next().id == track.id) {
                iterator.remove();
                removed = true;
                break;
            }
        }
        if (!removed) {
            liked.add(0, track);
        }
        final SharedPreferences preferences = prefs();
        if (preferences == null) {
            return;
        }
        try {
            final JSONArray array = new JSONArray();
            for (DeltamentSoundCloudClient.Track item : liked) {
                final JSONObject object = new JSONObject();
                object.put("id", item.id);
                object.put("title", item.title);
                object.put("artist", item.artist);
                if (item.artworkUrl != null) {
                    object.put("artwork", item.artworkUrl);
                }
                object.put("duration", item.durationMs);
                array.put(object);
            }
            preferences.edit().putString(KEY_LIKES, array.toString()).apply();
        } catch (Throwable e) {
            FileLog.e(e);
        }
    }

    // ----------------------------------------------------------------- artwork

    private void loadArtwork(String url, ImageView imageView) {
        if (TextUtils.isEmpty(url)) {
            imageView.setImageDrawable(null);
            return;
        }
        imageView.setTag(url);
        final Bitmap cached = artworkCache.get(url);
        if (cached != null) {
            imageView.setImageBitmap(cached);
            return;
        }
        Utilities.globalQueue.postRunnable(() -> {
            Bitmap bitmap = null;
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL(url).openConnection();
                connection.setConnectTimeout(15000);
                connection.setReadTimeout(20000);
                connection.setRequestProperty("User-Agent", "Mozilla/5.0");
                if (connection.getResponseCode() == HttpURLConnection.HTTP_OK) {
                    try (InputStream input = connection.getInputStream()) {
                        bitmap = BitmapFactory.decodeStream(input);
                    }
                }
            } catch (Throwable e) {
                FileLog.e(e);
            } finally {
                if (connection != null) {
                    connection.disconnect();
                }
            }
            final Bitmap result = bitmap;
            if (result != null) {
                artworkCache.put(url, result);
                AndroidUtilities.runOnUIThread(() -> {
                    if (url.equals(imageView.getTag())) {
                        imageView.setImageBitmap(result);
                    }
                });
            }
        });
    }

    // ------------------------------------------------------------- segment view

    private static class FixedHeightLinearLayout extends LinearLayout {

        private final int fixedHeight;

        FixedHeightLinearLayout(Context context, int fixedHeight) {
            super(context);
            this.fixedHeight = fixedHeight;
        }

        @Override
        protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
            super.onMeasure(widthMeasureSpec,
                    MeasureSpec.makeMeasureSpec(fixedHeight, MeasureSpec.EXACTLY));
        }
    }

    private static class SegmentRow extends View {

        interface Listener {
            void onSelected(int index);
        }

        private final String[] labels;
        private final Paint backgroundPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final Paint selectedPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final Paint textPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final Paint selectedTextPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final float radius;
        private int selected;
        private Listener listener;

        SegmentRow(Context context, String[] labels) {
            super(context);
            this.labels = labels;
            final float density = context.getResources().getDisplayMetrics().density;
            radius = 21 * density;
            backgroundPaint.setColor(COLOR_FIELD);
            selectedPaint.setColor(COLOR_BLUE);
            textPaint.setColor(COLOR_TEXT_SECONDARY);
            textPaint.setTextSize(15 * density);
            textPaint.setTextAlign(Paint.Align.CENTER);
            textPaint.setTypeface(Typeface.create(Typeface.DEFAULT, Typeface.NORMAL));
            selectedTextPaint.setColor(COLOR_TEXT);
            selectedTextPaint.setTextSize(15 * density);
            selectedTextPaint.setTextAlign(Paint.Align.CENTER);
            selectedTextPaint.setTypeface(Typeface.create(Typeface.DEFAULT, Typeface.BOLD));
        }

        void setListener(Listener listener) {
            this.listener = listener;
        }

        int getSelected() {
            return selected;
        }

        void setSelected(int index, boolean notify) {
            selected = Math.max(0, Math.min(labels.length - 1, index));
            invalidate();
            if (notify && listener != null) {
                listener.onSelected(selected);
            }
        }

        @Override
        protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
            final int width = MeasureSpec.getSize(widthMeasureSpec);
            final int height = (int) (42 * getResources().getDisplayMetrics().density);
            setMeasuredDimension(width, height);
        }

        @Override
        protected void onDraw(Canvas canvas) {
            final float width = getWidth();
            final float height = getHeight();
            final float inset = 2f * getResources().getDisplayMetrics().density;
            canvas.drawRoundRect(0, 0, width, height, radius, radius, backgroundPaint);

            final float segment = width / labels.length;
            final float left = selected * segment;
            final float right = left + segment;
            canvas.drawRoundRect(left + inset, inset, right - inset, height - inset,
                    (height - inset * 2) / 2f, (height - inset * 2) / 2f, selectedPaint);

            final float centerY = height / 2f - (textPaint.descent() + textPaint.ascent()) / 2f;
            for (int i = 0; i < labels.length; i++) {
                final Paint paint = i == selected ? selectedTextPaint : textPaint;
                canvas.drawText(labels[i], segment * i + segment / 2f, centerY, paint);
            }
        }

        @Override
        public boolean onTouchEvent(MotionEvent event) {
            if (event.getAction() == MotionEvent.ACTION_UP) {
                final int index = Math.max(0, Math.min(labels.length - 1,
                        (int) (event.getX() / (getWidth() / (float) labels.length))));
                if (index != selected) {
                    setSelected(index, true);
                }
                performClick();
                return true;
            }
            return event.getAction() == MotionEvent.ACTION_DOWN || super.onTouchEvent(event);
        }

        @Override
        public boolean performClick() {
            super.performClick();
            return true;
        }
    }
}
