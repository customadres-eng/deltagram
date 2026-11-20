// Verification badge artwork used by the direct-grant form.
//
// A third-party verification icon is a custom emoji document, not an arbitrary
// image: the server seeds these documents from the binary, and the client draws
// the document body through messages.getCustomEmojiDocuments. The panel therefore
// shows the real artwork (the same WebP files, copied into public/) instead of a
// generic glyph, and grants by document id.

export type VerificationIconAsset = {
  documentID: string;
  name: string;
  /** Filename stem under /verification-icons/. */
  slug: string;
};

export const verificationIconAssets: VerificationIconAsset[] = [
  { documentID: "5310000000000000111", name: "Major", slug: "major" },
  { documentID: "5310000000000000112", name: "Business", slug: "business" },
  { documentID: "5310000000000000113", name: "Diamond", slug: "diamond" },
  { documentID: "5310000000000000114", name: "Crown", slug: "crown" },
  { documentID: "5310000000000000115", name: "Star", slug: "star" },
  { documentID: "5310000000000000116", name: "Partner", slug: "partner" },
  { documentID: "5310000000000000117", name: "DeltaGram", slug: "deltagram" }
];

/** The Major badge is the default because it is the most requested one. */
export const DEFAULT_MARK_ICON_DOCUMENT_ID = verificationIconAssets[0].documentID;

const iconByDocumentID = new Map(
  verificationIconAssets.map((icon) => [icon.documentID, icon])
);

/** Resolves a seeded document id to the panel asset URL, if one exists. */
export function verificationIconSrc(documentID: string): string | undefined {
  const icon = iconByDocumentID.get(documentID);
  return icon ? `/verification-icons/${icon.slug}.webp` : undefined;
}

/** The readable label for a seeded document id. */
export function verificationIconName(documentID: string): string | undefined {
  return iconByDocumentID.get(documentID)?.name;
}

// The stored mark keeps the organization name, not a sentence: the server renders
// the sentence in the reader's language at projection time. This helper only
// previews that sentence in the panel's current language, and mirrors
// domain.LocalizedVerificationDescription for the languages the panel ships.
const verificationSentences: Record<string, string> = {
  en: "The account is verified by the organization “%s”.",
  ru: "Аккаунт верифицирован организацией «%s».",
  zh: "该账号已由组织“%s”验证。"
};

export function verificationDescription(organization: string, lang: string): string {
  const name = organization.trim();
  if (!name) return "";
  const format = verificationSentences[lang] ?? verificationSentences.en;
  return format.replace("%s", name);
}
