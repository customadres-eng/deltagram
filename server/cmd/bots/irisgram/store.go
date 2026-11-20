package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Settings are per-chat moderation rules. All durations are in seconds; zero
// means "forever" for ban/mute and "never expires" for warns.
type Settings struct {
	WarnLimit         int    `json:"warn_limit"`
	WarnMuteSeconds   int64  `json:"warn_mute_seconds"`
	WarnExpirySeconds int64  `json:"warn_expiry_seconds"`
	MuteSeconds       int64  `json:"mute_seconds"`
	BanSeconds        int64  `json:"ban_seconds"`
	WarnPunish        string `json:"warn_punish"` // mute | ban | kick
	Rules             string `json:"rules,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		WarnLimit:         3,
		WarnMuteSeconds:   3600,
		WarnExpirySeconds: 7 * 24 * 3600,
		MuteSeconds:       3600,
		BanSeconds:        0,
		WarnPunish:        "mute",
	}
}

func normalizeWarnPunish(v string) string {
	switch v {
	case "mute", "ban", "kick":
		return v
	default:
		return "mute"
	}
}

func (s Settings) normalized() Settings {
	d := DefaultSettings()
	if s.WarnLimit < 1 || s.WarnLimit > 20 {
		s.WarnLimit = d.WarnLimit
	}
	if s.WarnMuteSeconds < 0 {
		s.WarnMuteSeconds = d.WarnMuteSeconds
	}
	if s.WarnExpirySeconds < 0 {
		s.WarnExpirySeconds = d.WarnExpirySeconds
	}
	if s.MuteSeconds < 0 {
		s.MuteSeconds = d.MuteSeconds
	}
	if s.BanSeconds < 0 {
		s.BanSeconds = d.BanSeconds
	}
	s.WarnPunish = normalizeWarnPunish(s.WarnPunish)
	return s
}

type Warn struct {
	At        int64  `json:"at"`
	ExpiresAt int64  `json:"expires_at"`
	By        int64  `json:"by"`
	ByName    string `json:"by_name,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Moderator permission keys. They are stored as strings so older JSON state
// stays valid when a new permission is introduced.
const (
	PermBan        = "ban"
	PermUnban      = "unban"
	PermKick       = "kick"
	PermMute       = "mute"
	PermUnmute     = "unmute"
	PermWarn       = "warn"
	PermUnwarn     = "unwarn"
	PermResetWarns = "resetwarns"
	PermLists      = "lists"
)

// ModeratorPermissions controls what a bot-appointed moderator may do. Chat
// owner, Telegram admins and bot admins always have every right; this struct
// only applies to the moderator role.
type ModeratorPermissions struct {
	Ban        bool `json:"ban"`
	Unban      bool `json:"unban"`
	Kick       bool `json:"kick"`
	Mute       bool `json:"mute"`
	Unmute     bool `json:"unmute"`
	Warn       bool `json:"warn"`
	Unwarn     bool `json:"unwarn"`
	ResetWarns bool `json:"reset_warns"`
	Lists      bool `json:"lists"`
}

func DefaultModeratorPermissions() ModeratorPermissions {
	return ModeratorPermissions{
		Ban: true, Unban: true, Kick: true, Mute: true, Unmute: true,
		Warn: true, Unwarn: true, ResetWarns: true, Lists: true,
	}
}

// Set updates one permission and reports whether the key is known.
func (p *ModeratorPermissions) Set(permission string, value bool) bool {
	switch permission {
	case PermBan:
		p.Ban = value
	case PermUnban:
		p.Unban = value
	case PermKick:
		p.Kick = value
	case PermMute:
		p.Mute = value
	case PermUnmute:
		p.Unmute = value
	case PermWarn:
		p.Warn = value
	case PermUnwarn:
		p.Unwarn = value
	case PermResetWarns:
		p.ResetWarns = value
	case PermLists:
		p.Lists = value
	default:
		return false
	}
	return true
}

// Allows is the single source of truth used by command handlers.
func (p ModeratorPermissions) Allows(permission string) bool {
	switch permission {
	case PermBan:
		return p.Ban
	case PermUnban:
		return p.Unban
	case PermKick:
		return p.Kick
	case PermMute:
		return p.Mute
	case PermUnmute:
		return p.Unmute
	case PermWarn:
		return p.Warn
	case PermUnwarn:
		return p.Unwarn
	case PermResetWarns:
		return p.ResetWarns
	case PermLists:
		return p.Lists
	default:
		return false
	}
}

type Moderator struct {
	UserID      int64                `json:"user_id"`
	Name        string               `json:"name,omitempty"`
	Permissions ModeratorPermissions `json:"permissions"`
	AddedBy     int64                `json:"added_by,omitempty"`
	AddedAt     int64                `json:"added_at,omitempty"`
}

type Record struct {
	At     int64  `json:"at"`
	Until  int64  `json:"until"`
	By     int64  `json:"by"`
	ByName string `json:"by_name,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ChatState struct {
	Settings   Settings              `json:"settings"`
	Admins     []int64               `json:"admins,omitempty"`
	Moderators map[string]*Moderator `json:"moderators,omitempty"`
	Warns      map[string][]Warn     `json:"warns,omitempty"`
	Bans       map[string]Record     `json:"bans,omitempty"`
	Mutes      map[string]Record     `json:"mutes,omitempty"`
}

type State struct {
	Chats map[string]*ChatState `json:"chats"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	s := &Store{path: path, state: State{Chats: map[string]*ChatState{}}}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(data) > 0 {
			if err := json.Unmarshal(data, &s.state); err != nil {
				return nil, fmt.Errorf("parse state: %w", err)
			}
		}
	case os.IsNotExist(err):
	default:
		return nil, fmt.Errorf("read state: %w", err)
	}
	if s.state.Chats == nil {
		s.state.Chats = map[string]*ChatState{}
	}
	return s, nil
}

func chatKey(chatID int64) string { return strconv.FormatInt(chatID, 10) }

func newChatState() *ChatState {
	return &ChatState{
		Settings:   DefaultSettings(),
		Moderators: map[string]*Moderator{},
		Warns:      map[string][]Warn{},
		Bans:       map[string]Record{},
		Mutes:      map[string]Record{},
	}
}

func (s *Store) chatLocked(chatID int64) *ChatState {
	c, ok := s.state.Chats[chatKey(chatID)]
	if !ok || c == nil {
		c = newChatState()
		s.state.Chats[chatKey(chatID)] = c
	}
	if c.Moderators == nil {
		c.Moderators = map[string]*Moderator{}
	}
	if c.Warns == nil {
		c.Warns = map[string][]Warn{}
	}
	if c.Bans == nil {
		c.Bans = map[string]Record{}
	}
	if c.Mutes == nil {
		c.Mutes = map[string]Record{}
	}
	c.Settings = c.Settings.normalized()
	return c
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(&s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) mutate(chatID int64, fn func(*ChatState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.chatLocked(chatID))
	return s.saveLocked()
}

func (s *Store) Settings(chatID int64) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatLocked(chatID).Settings
}

func (s *Store) UpdateSettings(chatID int64, fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	fn(&c.Settings)
	c.Settings = c.Settings.normalized()
	if err := s.saveLocked(); err != nil {
		return c.Settings, err
	}
	return c.Settings, nil
}

func (s *Store) ResetSettings(chatID int64) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	c.Settings = DefaultSettings()
	if err := s.saveLocked(); err != nil {
		return c.Settings, err
	}
	return c.Settings, nil
}

// Rules returns the chat rules text configured by the owner.
func (s *Store) Rules(chatID int64) string {
	return s.Settings(chatID).Rules
}

// Stats is a point-in-time summary used by /stats.
type Stats struct {
	Admins     int
	Moderators int
	Warns      int
	Bans       int
	Mutes      int
}

func (s *Store) Stats(chatID int64) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	now := time.Now().Unix()
	var out Stats
	out.Admins = len(c.Admins)
	out.Moderators = len(c.Moderators)
	for _, list := range c.Warns {
		for _, w := range list {
			if w.ExpiresAt == 0 || w.ExpiresAt > now {
				out.Warns++
			}
		}
	}
	for _, rec := range c.Bans {
		if rec.Until == 0 || rec.Until > now {
			out.Bans++
		}
	}
	for _, rec := range c.Mutes {
		if rec.Until == 0 || rec.Until > now {
			out.Mutes++
		}
	}
	return out
}

func pruneWarns(list []Warn, now time.Time) []Warn {
	out := list[:0]
	for _, w := range list {
		if w.ExpiresAt > 0 && w.ExpiresAt <= now.Unix() {
			continue
		}
		out = append(out, w)
	}
	return out
}

func (s *Store) Warns(chatID, userID int64) []Warn {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	key := strconv.FormatInt(userID, 10)
	list := pruneWarns(c.Warns[key], time.Now())
	if len(list) == 0 {
		delete(c.Warns, key)
	} else {
		c.Warns[key] = list
	}
	out := make([]Warn, len(list))
	copy(out, list)
	return out
}

// AddWarn stores a warning and returns the new total. reached reports that the
// configured limit has been hit; the caller then applies the punishment and
// clears the list only after the punishment succeeded.
func (s *Store) AddWarn(chatID, userID int64, w Warn, limit int) (total int, reached bool, err error) {
	now := time.Now()
	err = s.mutate(chatID, func(c *ChatState) {
		key := strconv.FormatInt(userID, 10)
		list := pruneWarns(c.Warns[key], now)
		list = append(list, w)
		c.Warns[key] = list
		total = len(list)
		reached = limit > 0 && total >= limit
	})
	return total, reached, err
}

func (s *Store) RemoveLastWarn(chatID, userID int64) (left int, removed bool, err error) {
	err = s.mutate(chatID, func(c *ChatState) {
		key := strconv.FormatInt(userID, 10)
		list := pruneWarns(c.Warns[key], time.Now())
		if len(list) == 0 {
			delete(c.Warns, key)
			return
		}
		list = list[:len(list)-1]
		removed = true
		if len(list) == 0 {
			delete(c.Warns, key)
		} else {
			c.Warns[key] = list
		}
		left = len(list)
	})
	return left, removed, err
}

func (s *Store) ClearWarns(chatID, userID int64) error {
	return s.mutate(chatID, func(c *ChatState) {
		delete(c.Warns, strconv.FormatInt(userID, 10))
	})
}

func (s *Store) Admins(chatID int64) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	out := append([]int64(nil), c.Admins...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) IsAdmin(chatID, userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.chatLocked(chatID).Admins {
		if id == userID {
			return true
		}
	}
	return false
}

func (s *Store) AddAdmin(chatID, userID int64) (added bool, err error) {
	err = s.mutate(chatID, func(c *ChatState) {
		for _, id := range c.Admins {
			if id == userID {
				return
			}
		}
		c.Admins = append(c.Admins, userID)
		added = true
	})
	return added, err
}

func (s *Store) RemoveAdmin(chatID, userID int64) (removed bool, err error) {
	err = s.mutate(chatID, func(c *ChatState) {
		out := c.Admins[:0]
		for _, id := range c.Admins {
			if id == userID {
				removed = true
				continue
			}
			out = append(out, id)
		}
		c.Admins = out
	})
	return removed, err
}

// Moderators returns the chat moderators sorted by user ID.
func (s *Store) Moderators(chatID int64) []Moderator {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	out := make([]Moderator, 0, len(c.Moderators))
	for _, m := range c.Moderators {
		if m == nil || m.UserID == 0 {
			continue
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out
}

func (s *Store) Moderator(chatID, userID int64) (Moderator, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.chatLocked(chatID).Moderators[strconv.FormatInt(userID, 10)]
	if !ok || m == nil {
		return Moderator{}, false
	}
	return *m, true
}

func (s *Store) IsModerator(chatID, userID int64) bool {
	_, ok := s.Moderator(chatID, userID)
	return ok
}

// AddModerator creates a moderator with full default rights or updates the
// stored display name when the user is already a moderator. Existing rights
// are preserved on re-appointment.
func (s *Store) AddModerator(chatID, userID int64, name string, addedBy int64) (created bool, err error) {
	err = s.mutate(chatID, func(c *ChatState) {
		key := strconv.FormatInt(userID, 10)
		if m, ok := c.Moderators[key]; ok && m != nil {
			if strings.TrimSpace(name) != "" {
				m.Name = name
			}
			return
		}
		c.Moderators[key] = &Moderator{
			UserID:      userID,
			Name:        name,
			Permissions: DefaultModeratorPermissions(),
			AddedBy:     addedBy,
			AddedAt:     time.Now().Unix(),
		}
		created = true
	})
	return created, err
}

func (s *Store) RemoveModerator(chatID, userID int64) (removed bool, err error) {
	err = s.mutate(chatID, func(c *ChatState) {
		key := strconv.FormatInt(userID, 10)
		if _, ok := c.Moderators[key]; ok {
			delete(c.Moderators, key)
			removed = true
		}
	})
	return removed, err
}

func (s *Store) SetModeratorPermission(chatID, userID int64, permission string, value bool) (Moderator, bool, error) {
	var (
		updated Moderator
		found   bool
	)
	err := s.mutate(chatID, func(c *ChatState) {
		m, ok := c.Moderators[strconv.FormatInt(userID, 10)]
		if !ok || m == nil {
			return
		}
		if !m.Permissions.Set(permission, value) {
			return
		}
		updated = *m
		found = true
	})
	return updated, found, err
}

func (s *Store) ModeratorAllows(chatID, userID int64, permission string) bool {
	m, ok := s.Moderator(chatID, userID)
	if !ok {
		return false
	}
	return m.Permissions.Allows(permission)
}

func (s *Store) PutBan(chatID, userID int64, rec Record) error {
	return s.mutate(chatID, func(c *ChatState) {
		c.Bans[strconv.FormatInt(userID, 10)] = rec
	})
}

func (s *Store) DeleteBan(chatID, userID int64) error {
	return s.mutate(chatID, func(c *ChatState) {
		delete(c.Bans, strconv.FormatInt(userID, 10))
	})
}

func (s *Store) Ban(chatID, userID int64) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	key := strconv.FormatInt(userID, 10)
	rec, ok := c.Bans[key]
	if !ok {
		return Record{}, false
	}
	if rec.Until > 0 && rec.Until <= time.Now().Unix() {
		delete(c.Bans, key)
		return Record{}, false
	}
	return rec, true
}

func (s *Store) Mute(chatID, userID int64) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	key := strconv.FormatInt(userID, 10)
	rec, ok := c.Mutes[key]
	if !ok {
		return Record{}, false
	}
	if rec.Until > 0 && rec.Until <= time.Now().Unix() {
		delete(c.Mutes, key)
		return Record{}, false
	}
	return rec, true
}

func (s *Store) Bans(chatID int64) map[int64]Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	now := time.Now().Unix()
	out := map[int64]Record{}
	for key, rec := range c.Bans {
		if rec.Until > 0 && rec.Until <= now {
			delete(c.Bans, key)
			continue
		}
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		out[id] = rec
	}
	return out
}

func (s *Store) PutMute(chatID, userID int64, rec Record) error {
	return s.mutate(chatID, func(c *ChatState) {
		c.Mutes[strconv.FormatInt(userID, 10)] = rec
	})
}

func (s *Store) DeleteMute(chatID, userID int64) error {
	return s.mutate(chatID, func(c *ChatState) {
		delete(c.Mutes, strconv.FormatInt(userID, 10))
	})
}

func (s *Store) Mutes(chatID int64) map[int64]Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chatLocked(chatID)
	now := time.Now().Unix()
	out := map[int64]Record{}
	for key, rec := range c.Mutes {
		if rec.Until > 0 && rec.Until <= now {
			delete(c.Mutes, key)
			continue
		}
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		out[id] = rec
	}
	return out
}
