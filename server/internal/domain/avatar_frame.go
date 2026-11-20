package domain

import "strings"

// AvatarFrame is a cosmetic ring painted by the DeltaGram desktop client around
// a user's avatar. Frames are granted by operators through the admin panel and
// are delivered to clients through the public frame registry, so the client and
// the server share this exact id vocabulary.
type AvatarFrame string

const (
	AvatarFrameNone AvatarFrame = ""

	// Scratch is a set of sharp metallic scratches over a graphite ring.
	AvatarFrameScratch AvatarFrame = "scratch"
	// Bite is a chewed edge with dark bite holes and pale claw marks.
	AvatarFrameBite AvatarFrame = "bite"
	// Ember is a slow burning orange/red gradient ring.
	AvatarFrameEmber AvatarFrame = "ember"
	// Neon is a cyan-magenta double ring with a soft glow.
	AvatarFrameNeon AvatarFrame = "neon"
	// Gold is a polished gold gradient ring with a bright highlight.
	AvatarFrameGold AvatarFrame = "gold"
	// Sakura is a soft pink ring with falling petals.
	AvatarFrameSakura AvatarFrame = "sakura"
	// Void is a deep violet ring broken into orbiting shards.
	AvatarFrameVoid AvatarFrame = "void"
	// Frost is an icy blue ring with crystalline spikes.
	AvatarFrameFrost AvatarFrame = "frost"
	// Rainbow is a full-spectrum ring, the rarest of the set.
	AvatarFrameRainbow AvatarFrame = "rainbow"
)

// AllAvatarFrames lists every frame that the admin panel may grant and the
// desktop client knows how to paint, in display order.
var AllAvatarFrames = []AvatarFrame{
	AvatarFrameScratch,
	AvatarFrameBite,
	AvatarFrameEmber,
	AvatarFrameNeon,
	AvatarFrameGold,
	AvatarFrameSakura,
	AvatarFrameVoid,
	AvatarFrameFrost,
	AvatarFrameRainbow,
}

// ParseAvatarFrame normalises an operator-supplied frame id.
//
// An empty value (or "none") always means "remove the frame". Unknown ids are
// rejected so a typo in the admin panel never silently disables a paid feature.
func ParseAvatarFrame(value string) (AvatarFrame, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "none" || value == "default" {
		return AvatarFrameNone, true
	}
	frame := AvatarFrame(value)
	for _, known := range AllAvatarFrames {
		if frame == known {
			return frame, true
		}
	}
	return AvatarFrameNone, false
}

// Valid reports whether the frame id is a known, grantable frame or "none".
func (f AvatarFrame) Valid() bool {
	parsed, ok := ParseAvatarFrame(string(f))
	return ok && parsed == f
}
