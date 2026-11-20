package org.telegram.ui.Wallet;

import static org.telegram.messenger.AndroidUtilities.dp;

import android.content.Context;
import android.graphics.Color;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffColorFilter;
import android.graphics.drawable.GradientDrawable;
import android.media.MediaPlayer;
import android.media.PlaybackParams;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.widget.FrameLayout;
import android.widget.ImageButton;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.SeekBar;
import android.widget.TextView;

import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.MediaController;
import org.telegram.messenger.MessageObject;
import org.telegram.messenger.R;
import org.telegram.ui.ActionBar.ActionBar;
import org.telegram.ui.ActionBar.BaseFragment;
import org.telegram.ui.Components.LayoutHelper;

/**
 * DeltaGram's profile music player. It follows the reference full-screen player
 * (large rounded artwork, title, seek bar, transport row) but is intentionally
 * simpler and self-contained: one bundled ambient track, clear labels and a
 * bottom speed control.
 */
public class DeltamentMusicPlayerFragment extends BaseFragment {

    private final MessageObject playingMessage;
    private MediaPlayer player;
    private SeekBar seekBar;
    private TextView elapsed;
    private TextView total;
    private ImageButton playButton;
    private ImageView repeatButton;
    private ImageView shuffleButton;
    private TextView speedButton;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private boolean prepared;
    private float speed = 1f;
    private boolean repeat;
    private boolean shuffle;

    public DeltamentMusicPlayerFragment() {
        this(null);
    }

    public DeltamentMusicPlayerFragment(MessageObject message) {
        this.playingMessage = message;
    }

    private final Runnable ticker = new Runnable() {
        @Override
        public void run() {
            if (playingMessage != null) {
                MessageObject current = MediaController.getInstance().getPlayingMessageObject();
                if (current != null && current.getId() == playingMessage.getId()) {
                    int pos = (int) (current.audioProgress * seekBar.getMax());
                    seekBar.setProgress(pos);
                    elapsed.setText(fmt(pos));
                    updatePlay(!MediaController.getInstance().isMessagePaused());
                }
            } else if (player != null && prepared) {
                try {
                    int pos = player.getCurrentPosition();
                    seekBar.setProgress(pos);
                    elapsed.setText(fmt(pos));
                } catch (Exception ignored) {
                }
            }
            handler.postDelayed(this, 250);
        }
    };

    @Override
    public boolean onFragmentCreate() {
        return super.onFragmentCreate();
    }

    @Override
    public View createView(Context context) {
        actionBar.setBackButtonImage(R.drawable.ic_ab_back);
        actionBar.setAllowOverlayTitle(true);
        actionBar.setTitle("Музыка");
        actionBar.setBackgroundColor(0x00000000);
        actionBar.setTitleColor(Color.WHITE);
        actionBar.setItemsColor(Color.WHITE, false);
        actionBar.setItemsBackgroundColor(0x33ffffff, false);
        actionBar.setCastShadows(false);
        actionBar.setActionBarMenuOnItemClick(new ActionBar.ActionBarMenuOnItemClick() {
            @Override
            public void onItemClick(int id) {
                if (id == -1) {
                    finishFragment();
                }
            }
        });

        fragmentView = new FrameLayout(context);
        GradientDrawable bg = new GradientDrawable(
                GradientDrawable.Orientation.TL_BR,
                new int[]{0xff7b1f2c, 0xff4a121c, 0xff240a10});
        fragmentView.setBackground(bg);

        ScrollView scroll = new ScrollView(context);
        scroll.setFillViewport(true);
        scroll.setVerticalScrollBarEnabled(false);
        ((FrameLayout) fragmentView).addView(scroll, LayoutHelper.createFrame(
                LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));

        LinearLayout root = new LinearLayout(context);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(20), dp(8), dp(20), dp(20));
        scroll.addView(root, LayoutHelper.createScroll(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.TOP));

        TextView source = new TextView(context);
        source.setText("DELTAGRAM MUSIC");
        source.setTextSize(10);
        source.setLetterSpacing(0.18f);
        source.setTextColor(0x99ffffff);
        source.setGravity(Gravity.CENTER);
        root.addView(source, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 0, 0, 14));

        FrameLayout art = new FrameLayout(context);
        GradientDrawable artBg = new GradientDrawable(
                GradientDrawable.Orientation.TL_BR,
                new int[]{0xff8e2a39, 0xff5a1724, 0xff2c0c13});
        artBg.setCornerRadius(dp(28));
        art.setBackground(artBg);
        art.setClipToOutline(true);
        ImageView note = new ImageView(context);
        note.setImageResource(R.drawable.large_notes);
        note.setAlpha(0.38f);
        note.setColorFilter(new PorterDuffColorFilter(Color.WHITE, PorterDuff.Mode.SRC_IN));
        art.addView(note, LayoutHelper.createFrame(dp(88), dp(88), Gravity.CENTER));
        if (playingMessage != null && playingMessage.getDocument() != null) {
            org.telegram.ui.Components.BackupImageView cover = new org.telegram.ui.Components.BackupImageView(context);
            cover.setRoundRadius(dp(28));
            cover.setImage(org.telegram.messenger.ImageLocation.getForDocument(playingMessage.getDocument()), "260_260", (android.graphics.drawable.Drawable) null, playingMessage);
            art.addView(cover, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));
        }
        root.addView(art, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, 210, 0, 0, 0, 16));

        TextView title = new TextView(context);
        title.setText("Ambient Session");
        title.setTextSize(24);
        title.setTextColor(Color.WHITE);
        title.setTypeface(AndroidUtilities.bold());
        title.setSingleLine(true);
        title.setEllipsize(android.text.TextUtils.TruncateAt.END);
        root.addView(title, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        TextView artist = new TextView(context);
        artist.setText("DeltaGram Music · плейлист профиля");
        artist.setTextSize(14);
        artist.setTextColor(0xb3ffffff);
        artist.setSingleLine(true);
        artist.setEllipsize(android.text.TextUtils.TruncateAt.END);
        artist.setPadding(0, dp(4), 0, dp(16));
        root.addView(artist, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        seekBar = new SeekBar(context);
        seekBar.setMax(1);
        seekBar.getProgressDrawable().setColorFilter(
                new PorterDuffColorFilter(0xccffffff, PorterDuff.Mode.SRC_IN));
        seekBar.getThumb().setColorFilter(
                new PorterDuffColorFilter(Color.WHITE, PorterDuff.Mode.SRC_IN));
        root.addView(seekBar, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        LinearLayout times = new LinearLayout(context);
        times.setOrientation(LinearLayout.HORIZONTAL);
        elapsed = new TextView(context);
        elapsed.setText("0:00");
        elapsed.setTextSize(12);
        elapsed.setTextColor(0x99ffffff);
        total = new TextView(context);
        total.setText("0:00");
        total.setTextSize(12);
        total.setTextColor(0x99ffffff);
        times.addView(elapsed, LayoutHelper.createLinear(
                LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        times.addView(new View(context), LayoutHelper.createLinear(
                0, 1, 1f));
        times.addView(total, LayoutHelper.createLinear(
                LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        root.addView(times, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        LinearLayout controls = new LinearLayout(context);
        controls.setOrientation(LinearLayout.HORIZONTAL);
        controls.setGravity(Gravity.CENTER);
        controls.setPadding(0, dp(8), 0, dp(8));

        shuffleButton = iconButton(context, R.drawable.player_new_shuffle_off);
        controls.addView(shuffleButton, LayoutHelper.createLinear(dp(28), dp(28)));

        ImageButton prev = iconButton(context, R.drawable.ic_action_previous);
        prev.setOnClickListener(v -> seekTo(0));
        controls.addView(prev, LayoutHelper.createLinear(dp(28), dp(28)));

        playButton = iconButton(context, R.drawable.ic_action_play);
        GradientDrawable playBg = new GradientDrawable();
        playBg.setShape(GradientDrawable.OVAL);
        playBg.setColor(0xffffffff);
        playButton.setBackground(playBg);
        playButton.setColorFilter(new PorterDuffColorFilter(0xff6b1622, PorterDuff.Mode.SRC_IN));
        playButton.setOnClickListener(v -> toggle());
        controls.addView(playButton, LayoutHelper.createLinear(dp(44), dp(44), 0, 0, 0, 0));

        ImageButton next = iconButton(context, R.drawable.ic_action_next);
        next.setOnClickListener(v -> {
            if (player != null && prepared) {
                player.seekTo(0);
                if (!player.isPlaying()) toggle();
            }
        });
        controls.addView(next, LayoutHelper.createLinear(dp(28), dp(28)));

        repeatButton = iconButton(context, R.drawable.player_new_repeatall);
        controls.addView(repeatButton, LayoutHelper.createLinear(dp(28), dp(28)));
        root.addView(controls, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        LinearLayout bottom = new LinearLayout(context);
        bottom.setOrientation(LinearLayout.HORIZONTAL);
        bottom.setGravity(Gravity.CENTER);
        GradientDrawable bottomBg = new GradientDrawable();
        bottomBg.setCornerRadius(dp(22));
        bottomBg.setColor(0x1affffff);
        bottomBg.setStroke(dp(1), 0x33ffffff);
        bottom.setBackground(bottomBg);
        bottom.setPadding(dp(14), dp(8), dp(14), dp(8));

        TextView comment = new TextView(context);
        comment.setText("♫  Ambient");
        comment.setTextSize(13);
        comment.setTextColor(0xccffffff);
        bottom.addView(comment, LayoutHelper.createLinear(
                0, LayoutHelper.WRAP_CONTENT, 1f));
        speedButton = new TextView(context);
        speedButton.setText("1.0×");
        speedButton.setTextSize(13);
        speedButton.setTextColor(Color.WHITE);
        speedButton.setPadding(dp(12), dp(4), dp(12), dp(4));
        speedButton.setOnClickListener(v -> cycleSpeed());
        bottom.addView(speedButton, LayoutHelper.createLinear(
                LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        TextView eq = new TextView(context);
        eq.setText("☰");
        eq.setTextSize(18);
        eq.setTextColor(0xccffffff);
        bottom.addView(eq, LayoutHelper.createLinear(
                LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT));
        root.addView(bottom, LayoutHelper.createLinear(
                LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, 0, 8, 0, 0));

        shuffleButton.setOnClickListener(v -> {
            shuffle = !shuffle;
            shuffleButton.setAlpha(shuffle ? 1f : 0.55f);
            shuffleButton.setColorFilter(new PorterDuffColorFilter(shuffle ? 0xffffffff : 0x99ffffff, PorterDuff.Mode.SRC_IN));
        });
        repeatButton.setOnClickListener(v -> {
            repeat = !repeat;
            repeatButton.setAlpha(repeat ? 1f : 0.55f);
            repeatButton.setColorFilter(new PorterDuffColorFilter(repeat ? 0xffffffff : 0x99ffffff, PorterDuff.Mode.SRC_IN));
        });
        shuffleButton.setAlpha(0.55f);
        repeatButton.setAlpha(0.55f);

        seekBar.setOnSeekBarChangeListener(new SeekBar.OnSeekBarChangeListener() {
            @Override public void onProgressChanged(SeekBar seekBar, int progress, boolean fromUser) {
                if (fromUser) {
                    if (playingMessage != null && seekBar.getMax() > 0) {
                        MediaController.getInstance().seekToProgress(playingMessage, progress / (float) seekBar.getMax());
                    } else if (player != null && prepared) {
                        player.seekTo(progress);
                    }
                }
                elapsed.setText(fmt(progress));
            }
            @Override public void onStartTrackingTouch(SeekBar seekBar) {}
            @Override public void onStopTrackingTouch(SeekBar seekBar) {}
        });

        if (playingMessage != null) {
            final MediaController mc = MediaController.getInstance();
            mc.playMessage(playingMessage);
            title.setText(playingMessage.getMusicTitle());
            artist.setText(playingMessage.getMusicAuthor() + " · DeltaGram");
            int durationMs = (int) (playingMessage.getDuration() * 1000);
            seekBar.setMax(Math.max(1, durationMs));
            total.setText(fmt(durationMs));
            updatePlay(!mc.isMessagePaused());
        } else {
            // No saved track on this profile: never substitute an unrelated
            // bundled track. The screen stays open with an explanatory label.
            title.setText("Нет сохранённого трека");
            artist.setText("Добавьте музыку в профиль, чтобы она играла здесь");
            seekBar.setMax(1);
            total.setText("0:00");
            playButton.setEnabled(false);
            playButton.setAlpha(0.4f);
        }
        handler.post(ticker);
        return fragmentView;
    }

    private ImageButton iconButton(Context context, int res) {
        ImageButton b = new ImageButton(context);
        b.setImageResource(res);
        b.setBackground(null);
        b.setScaleType(ImageView.ScaleType.CENTER);
        b.setColorFilter(new PorterDuffColorFilter(Color.WHITE, PorterDuff.Mode.SRC_IN));
        return b;
    }

    private void toggle() {
        if (playingMessage != null) {
            final MediaController mc = MediaController.getInstance();
            if (mc.isPlayingMessage(playingMessage) && !mc.isMessagePaused()) {
                mc.pauseMessage(playingMessage);
                updatePlay(false);
            } else {
                mc.playMessage(playingMessage);
                updatePlay(true);
            }
            return;
        }
        if (player == null || !prepared) return;
        if (player.isPlaying()) {
            player.pause();
            updatePlay(false);
        } else {
            player.start();
            updatePlay(true);
        }
    }

    private void updatePlay(boolean playing) {
        if (playButton != null) {
            playButton.setImageResource(playing ? R.drawable.ic_action_pause : R.drawable.ic_action_play);
        }
    }

    private void seekTo(int ms) {
        if (player != null && prepared) {
            player.seekTo(Math.max(0, Math.min(ms, player.getDuration())));
        }
    }

    private void cycleSpeed() {
        float[] values = {1f, 1.25f, 1.5f, 0.75f};
        int idx = 0;
        for (int i = 0; i < values.length; i++) if (Math.abs(values[i] - speed) < 0.01f) idx = i;
        speed = values[(idx + 1) % values.length];
        speedButton.setText(String.format(java.util.Locale.US, "%.2f×", speed).replace("1.00", "1.0").replace("1.50", "1.5").replace("1.25", "1.25").replace("0.75", "0.75"));
        if (player != null && Build.VERSION.SDK_INT >= 23) {
            try {
                PlaybackParams params = player.getPlaybackParams();
                params.setSpeed(speed);
                player.setPlaybackParams(params);
            } catch (Exception ignored) {
            }
        }
    }

    private static String fmt(int ms) {
        int s = Math.max(0, ms / 1000);
        return String.format(java.util.Locale.US, "%d:%02d", s / 60, s % 60);
    }

    @Override
    public void onFragmentDestroy() {
        handler.removeCallbacksAndMessages(null);
        if (player != null) {
            try {
                player.stop();
            } catch (Exception ignored) {
            }
            player.release();
            player = null;
        }
        super.onFragmentDestroy();
    }
}
