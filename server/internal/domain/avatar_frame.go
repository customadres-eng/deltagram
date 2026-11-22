package domain

import "strings"

// AvatarFrame is an animated ring painted by DeltaGram around
// a user's avatar. Frames are granted by operators through the admin panel and
// are delivered to clients through the public frame registry, so the client and
// the server share this exact id vocabulary.
type AvatarFrame string

const (
	AvatarFrameNone AvatarFrame = ""

	AvatarFrameBlue         AvatarFrame = "blue"
	AvatarFrameFire         AvatarFrame = "fire"
	AvatarFrameGreen        AvatarFrame = "green"
	AvatarFrameHolographic  AvatarFrame = "holographic_glass_ring"
	AvatarFrameLiquidChrome AvatarFrame = "liquid_chrome_ring"
)

// AllAvatarFrames lists every frame that the admin panel may grant and the
// desktop client knows how to paint, in display order.
var AllAvatarFrames = []AvatarFrame{
	AvatarFrameBlue,
	AvatarFrameFire,
	AvatarFrameGreen,
	AvatarFrameHolographic,
	AvatarFrameLiquidChrome,
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
