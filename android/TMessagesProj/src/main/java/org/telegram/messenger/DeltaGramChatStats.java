package org.telegram.messenger;

import java.util.ArrayList;
import java.util.HashMap;

/**
 * Local "chat wrapped" statistics collected from the messages database.
 *
 * Nothing is sent to the server: the numbers are computed on the storage
 * thread from the last {@link #MAX_SCANNED} messages of a dialog.
 */
public class DeltaGramChatStats {

    public static final int MAX_SCANNED = 10000;

    public long dialogId;
    public int totalMessages;
    public int scannedMessages;
    public boolean truncated;

    public long firstMessageDate;
    public long lastMessageDate;

    public int outMessages;
    public int serviceMessages;

    public int photos;
    public int videos;
    public int voice;
    public int stickers;
    public int gifs;
    public int files;

    public final int[] byHour = new int[24];

    public final ArrayList<SenderStat> topSenders = new ArrayList<>();
    public final ArrayList<EmojiStat> topEmojis = new ArrayList<>();

    public long longestPauseSeconds;
    public long longestPauseStartDate;
    public long longestPauseEndDate;
    public long longestPauseFirstSenderId;
    public long longestPauseSecondSenderId;

    public interface Callback {
        void run(DeltaGramChatStats stats);
    }

    public static class SenderStat {
        public final long senderId;
        public int count;

        SenderStat(long senderId) {
            this.senderId = senderId;
        }
    }

    public static class EmojiStat {
        public final String emoji;
        public int count;

        EmojiStat(String emoji) {
            this.emoji = emoji;
        }
    }

    public int peakHour() {
        int best = 0;
        for (int i = 1; i < 24; i++) {
            if (byHour[i] > byHour[best]) {
                best = i;
            }
        }
        return best;
    }

    public int mediaTotal() {
        return photos + videos + voice + stickers + gifs + files;
    }

    /* ------------------------------------------------------------------ */
    /* Emoji scanning                                                      */
    /* ------------------------------------------------------------------ */

    public static void collectEmojis(String text, HashMap<String, EmojiStat> out) {
        if (text == null || text.length() == 0) {
            return;
        }
        int i = 0;
        while (i < text.length()) {
            final int cp = text.codePointAt(i);
            final int len = Character.charCount(cp);
            if (isEmojiCodePoint(cp)) {
                final StringBuilder sb = new StringBuilder();
                sb.appendCodePoint(cp);
                int j = i + len;
                while (j < text.length()) {
                    final int next = text.codePointAt(j);
                    if (next == 0xFE0F || next == 0x200D || next == 0x20E3 || isEmojiCodePoint(next)) {
                        sb.appendCodePoint(next);
                        j += Character.charCount(next);
                    } else {
                        break;
                    }
                }
                final String key = sb.toString();
                EmojiStat stat = out.get(key);
                if (stat == null) {
                    out.put(key, stat = new EmojiStat(key));
                }
                stat.count++;
                i = j;
            } else {
                i += len;
            }
        }
    }

    private static boolean isEmojiCodePoint(int cp) {
        return (cp >= 0x1F000 && cp <= 0x1FAFF)
                || (cp >= 0x2600 && cp <= 0x27BF)
                || (cp >= 0x2B00 && cp <= 0x2BFF)
                || (cp >= 0x1F1E6 && cp <= 0x1F1FF)
                || cp == 0x2764
                || cp == 0x203C
                || cp == 0x2049
                || cp == 0x2122
                || cp == 0x2139;
    }
}
