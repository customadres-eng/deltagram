package files

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/seed/verificationicons"
)

// VerificationIconSeedStats reports the bundled verification badge import.
type VerificationIconSeedStats struct {
	// Imported counts documents written by this run.
	Imported int
	// Skipped counts documents already present with their blob.
	Skipped int
}

// SeedVerificationIcons imports the static WebP custom emoji documents that back
// the operator's verification badges.
//
// The badge the client draws comes from messages.getCustomEmojiDocuments, so a
// catalogue row alone renders nothing. These bodies are bundled into the binary
// and imported at startup; the import is idempotent, leaving a document and its
// main blob untouched once they exist.
func (s *Service) SeedVerificationIcons(ctx context.Context) (VerificationIconSeedStats, error) {
	var stats VerificationIconSeedStats
	ref, err := hex.DecodeString(verificationicons.FileReference)
	if err != nil {
		return stats, fmt.Errorf("decode verification icon file reference: %w", err)
	}
	now := int(time.Now().Unix())
	documents := make([]domain.Document, 0, len(verificationicons.Icons))
	set := domain.StickerSet{
		ID:         verificationicons.SetID,
		AccessHash: verificationicons.SetAccessHash,
		ShortName:  "verification",
		Title:      "Verification",
		Hash:       5310000000000000003,
		Kind:       domain.StickerSetKindEmoji,
		Animated:   false,
		Emojis:     true,
		SortOrder:  100001,
	}
	for _, icon := range verificationicons.Icons {
		body, err := verificationicons.FS.ReadFile(icon.File)
		if err != nil {
			return stats, fmt.Errorf("read bundled verification icon %s: %w", icon.Name, err)
		}
		if len(body) == 0 {
			return stats, fmt.Errorf("bundled verification icon %s is empty", icon.Name)
		}
		locationKey := fmt.Sprintf("doc:%d", icon.DocumentID)
		if existing, found, err := s.media.GetDocument(ctx, icon.DocumentID); err != nil {
			return stats, err
		} else if found && existing.ID == icon.DocumentID {
			if _, _, err := s.media.GetFileBlob(ctx, locationKey); err == nil {
				documents = append(documents, existing)
				set.DocumentIDs = append(set.DocumentIDs, icon.DocumentID)
				set.Packs = append(set.Packs, domain.StickerPack{
					Emoticon:    icon.Alt,
					DocumentIDs: []int64{icon.DocumentID},
				})
				stats.Skipped++
				continue
			}
		}

		doc := domain.Document{
			ID:            icon.DocumentID,
			AccessHash:    verificationicons.DocumentAccessHash,
			FileReference: ref,
			Date:          now,
			MimeType:      "image/webp",
			Size:          int64(len(body)),
			DCID:          s.dc,
			Attributes: []domain.DocumentAttribute{
				{Kind: domain.DocAttrImageSize, W: 100, H: 100},
				{
					Kind:                 domain.DocAttrCustomEmoji,
					Alt:                  icon.Alt,
					StickerSetID:         verificationicons.SetID,
					StickerSetAccessHash: verificationicons.SetAccessHash,
				},
				{Kind: domain.DocAttrFilename, FileName: icon.File},
			},
		}

		objectKey, err := s.blobs.Put(ctx, body)
		if err != nil {
			return stats, err
		}
		if err := s.media.PutFileBlob(ctx, domain.FileBlob{
			LocationKey: locationKey,
			Backend:     domain.MediaBackend(s.blobs.Name()),
			ObjectKey:   objectKey,
			Size:        int64(len(body)),
			MimeType:    "image/webp",
		}); err != nil {
			return stats, err
		}
		s.prewarmSmallBlob(objectKey, body)
		if err := s.media.PutDocument(ctx, doc); err != nil {
			return stats, err
		}
		documents = append(documents, doc)
		set.DocumentIDs = append(set.DocumentIDs, icon.DocumentID)
		set.Packs = append(set.Packs, domain.StickerPack{
			Emoticon:    icon.Alt,
			DocumentIDs: []int64{icon.DocumentID},
		})
		stats.Imported++
	}
	set.Count = len(set.DocumentIDs)
	if err := s.media.PutStickerSet(ctx, set); err != nil {
		return stats, err
	}
	s.stickerSetCache.put(set, documents)
	return stats, nil
}
