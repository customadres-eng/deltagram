package main

import (
	"path/filepath"
	"testing"

	"github.com/iamxvbaba/td/tg"
)

func TestModeratorStoreLifecycleAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	const chatID = int64(-100123)
	const userID = int64(42)

	created, err := store.AddModerator(chatID, userID, "Иван", 7)
	if err != nil || !created {
		t.Fatalf("add moderator = created %v err %v", created, err)
	}
	m, ok := store.Moderator(chatID, userID)
	if !ok {
		t.Fatal("moderator not found after add")
	}
	if !m.Permissions.Ban || !m.Permissions.Warn || !m.Permissions.Lists {
		t.Fatalf("default permissions = %#v, want all enabled", m.Permissions)
	}

	updated, found, err := store.SetModeratorPermission(chatID, userID, PermBan, false)
	if err != nil || !found {
		t.Fatalf("set permission = found %v err %v", found, err)
	}
	if updated.Permissions.Ban {
		t.Fatal("ban permission still enabled")
	}
	if store.ModeratorAllows(chatID, userID, PermBan) {
		t.Fatal("ModeratorAllows(ban) = true after disable")
	}
	if !store.ModeratorAllows(chatID, userID, PermMute) {
		t.Fatal("ModeratorAllows(mute) = false, want true")
	}

	// Re-appointment keeps the edited rights.
	created, err = store.AddModerator(chatID, userID, "Иван Петров", 7)
	if err != nil || created {
		t.Fatalf("re-add moderator = created %v err %v", created, err)
	}
	if m, _ := store.Moderator(chatID, userID); m.Permissions.Ban || m.Name != "Иван Петров" {
		t.Fatalf("moderator after re-add = %#v", m)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if m, ok := reopened.Moderator(chatID, userID); !ok || m.Permissions.Ban {
		t.Fatalf("persisted moderator = %#v ok=%v", m, ok)
	}

	removed, err := reopened.RemoveModerator(chatID, userID)
	if err != nil || !removed {
		t.Fatalf("remove moderator = removed %v err %v", removed, err)
	}
	if reopened.IsModerator(chatID, userID) {
		t.Fatal("moderator still present after removal")
	}
}

func TestModeratorUnknownPermissionDoesNotChangeState(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.AddModerator(1, 2, "x", 0); err != nil {
		t.Fatalf("add moderator: %v", err)
	}
	if _, found, err := store.SetModeratorPermission(1, 2, "unknown", false); err != nil || found {
		t.Fatalf("unknown permission = found %v err %v, want false/nil", found, err)
	}
}

func TestParseDurationLooseAcceptsSeparateUnitWord(t *testing.T) {
	cases := []struct {
		args []string
		want int64
		rest []string
		ok   bool
	}{
		{[]string{"10", "минут", "флуд"}, 600, []string{"флуд"}, true},
		{[]string{"10м", "флуд"}, 600, []string{"флуд"}, true},
		{[]string{"2", "часа", "спам"}, 7200, []string{"спам"}, true},
		{[]string{"1", "день"}, 86400, nil, true},
		{[]string{"навсегда", "спам"}, 0, []string{"спам"}, true},
		{[]string{"флуд"}, 0, []string{"флуд"}, false},
	}
	for _, tc := range cases {
		got, rest, ok := parseDurationLoose(tc.args)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseDurationLoose(%v) = (%d, %v, %v), want (%d, %v, %v)", tc.args, got, rest, ok, tc.want, tc.rest, tc.ok)
			continue
		}
		if len(rest) != len(tc.rest) {
			t.Errorf("parseDurationLoose(%v) rest = %v, want %v", tc.args, rest, tc.rest)
			continue
		}
		for i := range rest {
			if rest[i] != tc.rest[i] {
				t.Errorf("parseDurationLoose(%v) rest = %v, want %v", tc.args, rest, tc.rest)
				break
			}
		}
	}
}

func TestMatchUserByUsernameFullNameAndPartialName(t *testing.T) {
	users := []*tg.User{
		{ID: 1, Username: "alice", FirstName: "Алиса", LastName: "Иванова"},
		{ID: 2, Username: "bob", FirstName: "Иван", LastName: "Петров"},
		{ID: 3, FirstName: "Иван", LastName: "Сидоров"},
	}
	if u := matchUser(users, "@alice"); u == nil || u.ID != 1 {
		t.Fatalf("match @alice = %#v", u)
	}
	if u := matchUser(users, "Иван Петров"); u == nil || u.ID != 2 {
		t.Fatalf("match full name = %#v", u)
	}
	if u := matchUser(users, "алиса"); u == nil || u.ID != 1 {
		t.Fatalf("match partial name = %#v", u)
	}
	if u := matchUser(users, "nobody"); u != nil {
		t.Fatalf("match nobody = %#v, want nil", u)
	}
}

func TestNormalizeCommandAcceptsRussianAndEnglishAliases(t *testing.T) {
	cases := map[string]string{
		"бан":        "ban",
		"мут":        "mute",
		"варн":       "warn",
		"размут":     "unmute",
		"модер":      "addmoder",
		"размодер":   "delmoder",
		"модеры":     "moderators",
		"моиправа":   "myrights",
		"айди":       "userid",
		"userid":     "userid",
		"moderators": "moderators",
	}
	for input, want := range cases {
		if got := normalizeCommand(input); got != want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", input, got, want)
		}
	}
}
