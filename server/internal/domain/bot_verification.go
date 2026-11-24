package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Third-party bot verification (core.telegram.org/api/bots/verification).
//
// A verifier bot marks a peer with its OWN icon and description. Clients render
// that icon before the name and the description in the profile, which is
// deliberately different from the official checkmark: only the operator grants the
// platform flag (see verification.go), and the two mechanisms never read each
// other's state.
//
// Layer 228 shapes this feature projects onto:
//
//	botVerification#f93cd45c bot_id:long icon:long description:string
//	botVerifierSettings#b0cd6617 flags:# can_modify_custom_description:flags.1?true
//	    icon:long company:string custom_description:flags.0?string
//	bots.setCustomVerification#8b89dfbd flags:# enabled:flags.1?true
//	    bot:flags.0?InputUser peer:InputPeer custom_description:flags.2?string = Bool
//	user#b1b8cc83        bot_verification_icon:flags2.14?long
//	channel#d49f34c6     bot_verification_icon:flags2.13?long
//	userFull#6cbe645     bot_verification:flags2.12?BotVerification
//	channelFull#a04e8d3a bot_verification:flags2.17?BotVerification
//	chatInvite#5c9d3702  bot_verification:flags.13?BotVerification
//	botInfo#4d8a0299     verifier_settings:flags.9?BotVerifierSettings
//
// The icon is a custom emoji document id. Clients resolve it through
// messages.getCustomEmojiDocuments, so an id that names no fetchable document
// renders as nothing at all -- which is why the catalogue is validated against
// real documents rather than accepting arbitrary numbers.
const (
	// MaxVerifierCompanyLength bounds botVerifierSettings.company.
	MaxVerifierCompanyLength = 128
	// MaxCustomVerificationDescriptionLength is the app-configured limit for text
	// supplied by a verifier (bot_verification_description_length_limit).
	MaxCustomVerificationDescriptionLength = 128
	// MaxBotVerificationDescriptionLength bounds the final wire description. It
	// may exceed the custom-input limit because the server-generated fallback
	// includes the organization name.
	MaxBotVerificationDescriptionLength = 1024
	// MaxVerificationIconNameLength bounds an operator-facing catalogue label.
	MaxVerificationIconNameLength = 128
	// MaxVerificationOrganizationLength bounds the organization name a mark is
	// issued for. The name is stored separately from the sentence so the sentence
	// can be rendered in the viewer's language.
	MaxVerificationOrganizationLength = 128
	// MaxCustomVerificationReasonLength bounds an applicant's stated reason.
	MaxCustomVerificationReasonLength = 4096
	// MaxCustomVerificationNoteLength bounds an operator-only note.
	MaxCustomVerificationNoteLength = 8192
	// MaxVerifierGrantReasonLength bounds the reason a bot was made a verifier.
	MaxVerifierGrantReasonLength = 1024
	// MaxCustomVerificationsPerVerifier bounds how many peers one verifier may
	// mark. Verifier status is granted per deployment, not earned per peer, so an
	// unbounded verifier would be an unbounded badge printer.
	MaxCustomVerificationsPerVerifier = 10000
)

var (
	// ErrVerifierNotFound reports a bot without verifier status.
	ErrVerifierNotFound = errors.New("bot verifier settings not found")
	// ErrVerifierForbidden reports a bot that may not verify: no verifier row, or a
	// row the operator disabled. It maps to BOT_VERIFIER_FORBIDDEN.
	ErrVerifierForbidden = errors.New("bot is not allowed to verify peers")
	// ErrVerifierSettingsInvalid rejects a malformed verifier configuration.
	ErrVerifierSettingsInvalid = errors.New("bot verifier settings invalid")
	// ErrVerifierDescriptionForbidden reports a per-peer description supplied by a
	// verifier whose can_modify_custom_description is false.
	ErrVerifierDescriptionForbidden = errors.New("verifier may not set a custom description")
	// ErrVerificationIconNotFound reports an icon missing from the catalogue.
	ErrVerificationIconNotFound = errors.New("verification icon not found")
	// ErrVerificationIconInactive rejects an icon the operator retired.
	ErrVerificationIconInactive = errors.New("verification icon inactive")
	// ErrVerificationIconInvalid rejects a malformed icon record.
	ErrVerificationIconInvalid = errors.New("verification icon invalid")
	// ErrCustomVerificationNotFound reports a peer this verifier has not marked.
	ErrCustomVerificationNotFound = errors.New("custom verification not found")
	// ErrCustomVerificationLimit reports the per-verifier bound.
	ErrCustomVerificationLimit = errors.New("custom verification limit reached")
	// ErrCustomVerificationTargetInvalid rejects an unverifiable peer.
	ErrCustomVerificationTargetInvalid = errors.New("custom verification target invalid")
	// ErrCustomVerificationRequestNotFound reports a missing application.
	ErrCustomVerificationRequestNotFound = errors.New("custom verification request not found")
	// ErrCustomVerificationRequestExists reports a pending application already
	// occupying the (verifier, peer) pair.
	ErrCustomVerificationRequestExists = errors.New("custom verification request already pending")
	// ErrCustomVerificationRequestInvalid rejects a malformed application.
	ErrCustomVerificationRequestInvalid = errors.New("custom verification request invalid")
	// ErrCustomVerificationVersionConflict reports a lost optimistic-locking race.
	ErrCustomVerificationVersionConflict = errors.New("custom verification changed concurrently")
)

// VerificationIcon is one catalogue entry: a custom emoji document usable as a
// verifier's mark.
type VerificationIcon struct {
	ID         int64
	DocumentID int64
	// OwnerBotID is zero for a shared entry and a bot id when the operator
	// reserved the icon for one verifier.
	OwnerBotID int64
	Name       string
	Active     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Validate checks the catalogue entry shape.
func (i VerificationIcon) Validate() error {
	if i.DocumentID <= 0 {
		return ErrVerificationIconInvalid
	}
	name := strings.TrimSpace(i.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxVerificationIconNameLength {
		return ErrVerificationIconInvalid
	}
	if i.OwnerBotID < 0 {
		return ErrVerificationIconInvalid
	}
	return nil
}

// UsableBy reports whether a verifier may mark peers with this icon.
func (i VerificationIcon) UsableBy(botID int64) bool {
	return i.Active && (i.OwnerBotID == 0 || i.OwnerBotID == botID)
}

// BotVerifierSettings is a bot's verifier status, projected onto
// botVerifierSettings#b0cd6617 and consulted by bots.setCustomVerification.
type BotVerifierSettings struct {
	BotID              int64
	IconDocumentID     int64
	CompanyName        string
	DefaultDescription string
	// CanModifyCustomDescription mirrors flags.1 of the TL constructor: when false
	// the verifier may only apply DefaultDescription.
	CanModifyCustomDescription bool
	// Enabled is the operator's kill switch. A disabled verifier keeps its row and
	// its granted marks but can no longer mark anything new, and the settings stop
	// being projected into botInfo.
	Enabled     bool
	GrantedBy   string
	GrantReason string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int64
}

// Validate checks the verifier configuration.
func (s BotVerifierSettings) Validate() error {
	if s.BotID <= 0 || s.IconDocumentID <= 0 {
		return ErrVerifierSettingsInvalid
	}
	company := strings.TrimSpace(s.CompanyName)
	if company == "" || utf8.RuneCountInString(company) > MaxVerifierCompanyLength {
		return ErrVerifierSettingsInvalid
	}
	if utf8.RuneCountInString(s.DefaultDescription) > MaxCustomVerificationDescriptionLength {
		return ErrVerifierSettingsInvalid
	}
	if len(s.GrantedBy) > 128 || utf8.RuneCountInString(s.GrantReason) > MaxVerifierGrantReasonLength {
		return ErrVerifierSettingsInvalid
	}
	return nil
}

// DescriptionFor resolves the description a mark carries: the verifier-supplied
// one when allowed, the configured default, or the protocol-defined generated
// fallback.
func (s BotVerifierSettings) DescriptionFor(custom string) (string, error) {
	custom = strings.TrimSpace(custom)
	if custom == "" {
		if fallback := strings.TrimSpace(s.DefaultDescription); fallback != "" {
			return fallback, nil
		}
		return fmt.Sprintf(`Was verified by organization "%s"`, strings.TrimSpace(s.CompanyName)), nil
	}
	if !s.CanModifyCustomDescription {
		return "", ErrVerifierDescriptionForbidden
	}
	if utf8.RuneCountInString(custom) > MaxCustomVerificationDescriptionLength {
		return "", ErrCustomVerificationRequestInvalid
	}
	return custom, nil
}

// CustomVerification is one granted third-party mark.
type CustomVerification struct {
	ID            int64
	VerifierBotID int64
	Peer          Peer
	// IconDocumentID is denormalised at grant time so the mark keeps rendering the
	// icon it was granted with even after the verifier changes its own.
	IconDocumentID int64
	// Description is the exact stored sentence. It is the fallback when
	// OrganizationName is empty (marks granted before localization, or marks that
	// carry free-form text) and the value shipped to clients that do not advertise
	// a language.
	Description string
	// OrganizationName, when set, is rendered into a language-specific sentence at
	// projection time. The client's language comes from initConnection.lang_code,
	// so one stored mark reads naturally in every viewer's language.
	OrganizationName string
	GrantedByUserID  int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Version          int64
}

// BotVerification is the TL projection (botVerification#f93cd45c).
type BotVerification struct {
	BotID       int64
	Icon        int64
	Description string
	// Entities carries custom emoji spans in Description. It is how several
	// verification marks share the single bot_verification slot: the first mark is
	// the struct's icon and every additional mark contributes an inline custom
	// emoji entity plus its sentence.
	Entities []MessageEntity
}

// Projection returns the botVerification payload for full peer objects.
func (v CustomVerification) Projection() BotVerification {
	description := v.Description
	if v.Peer.Type == PeerTypeChannel {
		if organization := strings.TrimSpace(v.OrganizationName); organization != "" {
			description = LocalizedChannelVerificationDescription("en", organization)
		} else {
			description = channelVerificationDescription(description)
		}
	}
	return BotVerification{
		BotID:       v.VerifierBotID,
		Icon:        v.IconDocumentID,
		Description: description,
	}
}

// ProjectionForLanguage returns the payload with the description rendered in the
// viewer's language when the mark carries an organization name.
func (v CustomVerification) ProjectionForLanguage(langCode string) BotVerification {
	description := v.Description
	if organization := strings.TrimSpace(v.OrganizationName); organization != "" {
		if v.Peer.Type == PeerTypeChannel {
			description = LocalizedChannelVerificationDescription(langCode, organization)
		} else {
			description = LocalizedVerificationDescription(langCode, organization)
		}
	} else if v.Peer.Type == PeerTypeChannel {
		description = channelVerificationDescription(description)
	}
	return BotVerification{
		BotID:       v.VerifierBotID,
		Icon:        v.IconDocumentID,
		Description: description,
	}
}

// channelVerificationDescription rewrites the legacy stored "account" sentence
// for marks granted to channels before channel-specific wording existed.
func channelVerificationDescription(description string) string {
	replacements := []struct{ old, new string }{
		{"Аккаунт", "Канал"},
		{"аккаунт", "канал"},
		{"Акаунт", "Канал"},
		{"акаунт", "канал"},
		{"Account", "Channel"},
		{"account", "channel"},
	}
	for _, replacement := range replacements {
		description = strings.ReplaceAll(description, replacement.old, replacement.new)
	}
	return description
}

// LocalizedVerificationDescription renders the protocol sentence for one
// organization in the viewer's language.
//
// The sentence is deliberately server-side: clients draw bot_verification.description
// verbatim, so a fixed stored string would read in the language of whoever granted
// the mark rather than in the language of the person reading the profile. Unknown
// languages fall back to English, which is also what the client sees when it does
// not advertise a language.
func LocalizedVerificationDescription(langCode, organization string) string {
	organization = strings.TrimSpace(organization)
	format, ok := verificationDescriptionFormats[normalizeVerificationLanguage(langCode)]
	if !ok {
		format = verificationDescriptionFormats["en"]
	}
	return fmt.Sprintf(format, organization)
}

// verificationDescriptionFormats maps a primary language subtag to the sentence
// around the organization name. Keep the organization inside the same quoting
// convention each language uses for organization names.
// channelVerificationDescriptionFormats is the channel-specific sentence. The
// protocol description is rendered by the client verbatim, so a channel mark
// must not say "account".
var channelVerificationDescriptionFormats = map[string]string{
	"en": `The channel is verified by the organization "%s".`,
	"ru": `Канал верифицирован организацией «%s».`,
	"uk": `Канал верифіковано організацією «%s».`,
}

// LocalizedChannelVerificationDescription is the channel counterpart of
// LocalizedVerificationDescription.
func LocalizedChannelVerificationDescription(langCode, organization string) string {
	organization = strings.TrimSpace(organization)
	format, ok := channelVerificationDescriptionFormats[normalizeVerificationLanguage(langCode)]
	if !ok {
		format = channelVerificationDescriptionFormats["en"]
	}
	return fmt.Sprintf(format, organization)
}

var verificationDescriptionFormats = map[string]string{
	"en": `The account is verified by the organization "%s".`,
	"ru": `Аккаунт верифицирован организацией «%s».`,
	"uk": `Акаунт верифіковано організацією «%s».`,
	"be": `Акаунт верыфікаваны арганізацыяй «%s».`,
	"kk": `Аккаунт «%s» ұйымымен верификацияланған.`,
	"zh": `该账号已由组织“%s”验证。`,
	"de": `Das Konto wurde von der Organisation „%s“ verifiziert.`,
	"fr": `Le compte est vérifié par l’organisation « %s ».`,
	"es": `La cuenta está verificada por la organización «%s».`,
	"it": `L’account è verificato dall’organizzazione «%s».`,
	"pt": `A conta foi verificada pela organização «%s».`,
	"nl": `Het account is geverifieerd door de organisatie “%s”.`,
	"tr": `Hesap "%s" kuruluşu tarafından doğrulandı.`,
	"pl": `Konto zostało zweryfikowane przez organizację „%s”.`,
	"cs": `Účet byl ověřen organizací „%s“.`,
	"ro": `Contul este verificat de organizația „%s”.`,
	"bg": `Акаунтът е верифициран от организация „%s“.`,
	"sr": `Налог је верификовала организација „%s”.`,
	"hr": `Račun je verificirala organizacija „%s”.`,
	"ar": `تم التحقق من الحساب بواسطة مؤسسة «%s».`,
	"fa": `حساب توسط سازمان «%s» تأیید شده است.`,
	"he": `החשבון אומת על ידי הארגון «%s».`,
	"ja": `アカウントは「%s」という組織によって認証されています。`,
	"ko": `계정은 «%s» 조직에 의해 인증되었습니다.`,
	"hi": `खाता "%s" संगठन द्वारा सत्यापित है।`,
	"id": `Akun telah diverifikasi oleh organisasi «%s».`,
	"vi": `Tài khoản đã được xác minh bởi tổ chức «%s».`,
	"th": `บัญชีได้รับการยืนยันโดยองค์กร «%s»`,
	"ms": `Akaun telah disahkan oleh organisasi «%s».`,
	"az": `Hesab "%s" təşkilatı tərəfindən təsdiqlənib.`,
	"hy": `Հաշիվը հաստատված է «%s» կազմակերպության կողմից։`,
	"ka": `ანგარიში დადასტურებულია ორგანიზაცია «%s»-ის მიერ.`,
	"uz": `Hisob "%s" tashkiloti tomonidan tasdiqlangan.`,
}

// normalizeVerificationLanguage reduces a client language tag such as ru-RU,
// zh_hans or pt-BR to the primary subtag used by the format table.
func normalizeVerificationLanguage(langCode string) string {
	langCode = strings.ToLower(strings.TrimSpace(langCode))
	langCode = strings.ReplaceAll(langCode, "_", "-")
	if index := strings.IndexByte(langCode, '-'); index >= 0 {
		langCode = langCode[:index]
	}
	return langCode
}

// Validate checks the mark shape.
func (v CustomVerification) Validate() error {
	if v.VerifierBotID <= 0 || v.IconDocumentID <= 0 {
		return ErrCustomVerificationTargetInvalid
	}
	if !validCustomVerificationPeer(v.Peer) {
		return ErrCustomVerificationTargetInvalid
	}
	if utf8.RuneCountInString(v.Description) > MaxBotVerificationDescriptionLength {
		return ErrCustomVerificationRequestInvalid
	}
	if utf8.RuneCountInString(v.OrganizationName) > MaxVerificationOrganizationLength {
		return ErrCustomVerificationRequestInvalid
	}
	return nil
}

func validCustomVerificationPeer(peer Peer) bool {
	switch peer.Type {
	case PeerTypeUser, PeerTypeChannel:
		return peer.ID > 0
	default:
		return false
	}
}

// CustomVerificationRequestStatus is the application lifecycle in front of a mark.
type CustomVerificationRequestStatus string

const (
	CustomVerificationPending  CustomVerificationRequestStatus = "pending"
	CustomVerificationApproved CustomVerificationRequestStatus = "approved"
	CustomVerificationRejected CustomVerificationRequestStatus = "rejected"
	CustomVerificationRevoked  CustomVerificationRequestStatus = "revoked"
)

// Valid reports whether the status is modelled.
func (s CustomVerificationRequestStatus) Valid() bool {
	switch s {
	case CustomVerificationPending, CustomVerificationApproved,
		CustomVerificationRejected, CustomVerificationRevoked:
		return true
	default:
		return false
	}
}

// CanTransitionCustomVerificationStatus is the status machine. A revoked mark is
// reached only from approved, which keeps "revoked" meaning "was verified once".
func CanTransitionCustomVerificationStatus(from, to CustomVerificationRequestStatus) bool {
	if !from.Valid() || !to.Valid() || from == to {
		return false
	}
	switch from {
	case CustomVerificationPending:
		return to == CustomVerificationApproved || to == CustomVerificationRejected
	case CustomVerificationApproved:
		return to == CustomVerificationRevoked
	default:
		return false
	}
}

// CustomVerificationRequest is an application filed with a verifier bot.
type CustomVerificationRequest struct {
	ID                   int64
	VerifierBotID        int64
	ApplicantUserID      int64
	Peer                 Peer
	PeerTitle            string
	PeerUsername         string
	Reason               string
	RequestedDescription string
	Status               CustomVerificationRequestStatus
	DecidedBy            string
	DecisionReason       string
	InternalNote         string
	CorrelationID        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ApprovedAt           time.Time
	RejectedAt           time.Time
	Version              int64
}

// Validate checks the application shape.
func (r CustomVerificationRequest) Validate() error {
	if r.VerifierBotID <= 0 || r.ApplicantUserID <= 0 || !validCustomVerificationPeer(r.Peer) {
		return ErrCustomVerificationRequestInvalid
	}
	if !r.Status.Valid() {
		return ErrCustomVerificationRequestInvalid
	}
	if utf8.RuneCountInString(r.Reason) > MaxCustomVerificationReasonLength ||
		utf8.RuneCountInString(r.RequestedDescription) > MaxCustomVerificationDescriptionLength ||
		utf8.RuneCountInString(r.InternalNote) > MaxCustomVerificationNoteLength {
		return ErrCustomVerificationRequestInvalid
	}
	if r.Status == CustomVerificationRejected && strings.TrimSpace(r.DecisionReason) == "" {
		return ErrVerificationReasonRequired
	}
	return nil
}

// SetCustomVerificationRequest is the bots.setCustomVerification payload after the
// RPC edge has resolved the caller, the verifier bot and the target peer.
type SetCustomVerificationRequest struct {
	VerifierBotID int64
	Peer          Peer
	// Enabled false revokes the mark this verifier granted; the request then
	// carries no description.
	Enabled bool
	// CustomDescription is the verifier-supplied per-peer text. It is only honoured
	// when the verifier's settings allow it.
	CustomDescription string
	// CallerUserID is the account that invoked the RPC: the bot itself or its owner.
	CallerUserID int64
}

// Validate checks the request shape without consulting stored state.
func (r SetCustomVerificationRequest) Validate() error {
	if r.VerifierBotID <= 0 || !validCustomVerificationPeer(r.Peer) {
		return ErrCustomVerificationTargetInvalid
	}
	if utf8.RuneCountInString(r.CustomDescription) > MaxCustomVerificationDescriptionLength {
		return ErrCustomVerificationRequestInvalid
	}
	if !r.Enabled && strings.TrimSpace(r.CustomDescription) != "" {
		// Revocation with a description is a caller bug worth reporting rather than
		// silently ignoring: it usually means enabled was left unset by mistake.
		return ErrCustomVerificationRequestInvalid
	}
	return nil
}

// CustomVerificationFilter bounds an admin listing query.
type CustomVerificationFilter struct {
	VerifierBotID int64
	PeerType      PeerType
	PeerID        int64
	Query         string
	BeforeID      int64
	Limit         int
}

// CustomVerificationRequestFilter bounds a review-queue query.
type CustomVerificationRequestFilter struct {
	Statuses      []CustomVerificationRequestStatus
	VerifierBotID int64
	PeerType      PeerType
	Query         string
	BeforeID      int64
	Limit         int
}
