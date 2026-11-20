// Package verificationicons bundles the operator's verification badge artwork
// into the server binary.
//
// A third-party verification icon is a custom emoji document, not an arbitrary
// URL: clients resolve it through messages.getCustomEmojiDocuments and then draw
// the document body. These are static WebP custom emoji, so the badge is the real
// artwork supplied by the operator instead of a Unicode emoji glyph.
//
// Seeding from the binary keeps the set available on every deployment, exactly
// like internal/seed/freeze does for the frozen-account mark.
package verificationicons

import "embed"

// FS serves the bundled static WebP custom emoji bodies.
//
//go:embed *.webp
var FS embed.FS

// SetID identifies the "verification" custom emoji set that owns the documents.
const SetID int64 = 5310000000000000001

// SetAccessHash pairs with SetID.
const SetAccessHash int64 = 5310000000000000002

// DocumentAccessHash is shared by the bundled documents. Access hashes only
// authorise the client to reference the document; the ids are what the badges
// carry, so a stable shared value keeps the seed deterministic.
const DocumentAccessHash int64 = 5310000000000000999

// FileReference is the opaque reference clients echo back on upload.getFile.
const FileReference = "00d1f6f438a12476426a7268240fc54f"

// Icon is one bundled badge: its stable document identity and its catalogue name.
type Icon struct {
	// Name is the operator-facing catalogue label and the filename stem.
	Name string
	// DocumentID is the custom emoji document id the badge carries.
	DocumentID int64
	// File is the embedded WebP body.
	File string
	// Alt is the emoji the custom emoji attribute maps to.
	Alt string
	// Slug is the admin-panel asset name under /verification-icons/.
	Slug string
}

// Icons is the seeded catalogue, in the order the admin panel should present it.
var Icons = []Icon{
	{Name: "Major", DocumentID: 5310000000000000111, File: "major.webp", Alt: "🏅", Slug: "major"},
	{Name: "Business", DocumentID: 5310000000000000112, File: "business.webp", Alt: "💼", Slug: "business"},
	{Name: "Diamond", DocumentID: 5310000000000000113, File: "diamond.webp", Alt: "💎", Slug: "diamond"},
	{Name: "Crown", DocumentID: 5310000000000000114, File: "crown.webp", Alt: "👑", Slug: "crown"},
	{Name: "Star", DocumentID: 5310000000000000115, File: "star.webp", Alt: "⭐", Slug: "star"},
	{Name: "Partner", DocumentID: 5310000000000000116, File: "partner.webp", Alt: "🤝", Slug: "partner"},
	{Name: "DeltaGram", DocumentID: 5310000000000000117, File: "deltagram.webp", Alt: "🇩", Slug: "deltagram"},
}
