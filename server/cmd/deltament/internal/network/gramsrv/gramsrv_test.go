package gramsrv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
)

// фейковый gramsrv, запоминает какие ручки дёргали
func fake(t *testing.T, handle func(path string, body map[string]any) (int, any)) (*Client, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		calls = append(calls, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		status, out := handle(r.URL.Path, body)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func TestResolvePhone(t *testing.T) {
	c, _ := fake(t, func(_ string, body map[string]any) (int, any) {
		if body["phone"] == "+79990000000" {
			return 200, map[string]any{"found": true, "user_id": 42}
		}
		return 200, map[string]any{"found": false, "user_id": 0}
	})
	id, err := c.ResolvePhone(context.Background(), "+79990000000")
	if err != nil || id != 42 {
		t.Fatalf("got %d %v", id, err)
	}
	if _, err := c.ResolvePhone(context.Background(), "+70000000000"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

// если юзернейм уже есть на сервере, делаем transfer вместо mint
func TestGrantUsernameFallsBackToTransfer(t *testing.T) {
	c, calls := fake(t, func(path string, _ map[string]any) (int, any) {
		if path == "/v1/collectible-usernames/mint" {
			return 400, map[string]any{"status": "failed", "error": "username occupied", "code": "USERNAME_OCCUPIED"}
		}
		return 200, map[string]any{"status": "executed"}
	})
	err := c.GrantUsername(context.Background(), network.Op{Key: "item:1"}, 42, "lunar")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/v1/collectible-usernames/mint", "/v1/collectible-usernames/transfer"}
	if len(*calls) != 2 || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Fatalf("calls = %v", *calls)
	}
}

func TestCommandIDIsStable(t *testing.T) {
	c, _ := New("http://x", "tok")
	a := c.meta(network.Op{Key: "item:7"}).CommandID
	b := c.meta(network.Op{Key: "item:7"}).CommandID
	if a != b || a == c.meta(network.Op{Key: "item:8"}).CommandID {
		t.Fatal("command id must depend only on op key")
	}
}

// The Admin API keys idempotency globally by command_id, so two actions that
// share an operation key (delete channel, then mint its username into escrow)
// must not share a command id.
func TestCommandIDIsActionScoped(t *testing.T) {
	c, _ := New("http://x", "tok")
	op := network.Op{Key: "escrow:11"}
	mint := c.metaAction(op, "escrow-mint-username").CommandID
	remove := c.metaAction(op, "escrow-revoke-username").CommandID
	if mint == remove {
		t.Fatal("different actions must use different command ids")
	}
	if mint != c.metaAction(op, "escrow-mint-username").CommandID {
		t.Fatal("command id must stay stable across retries")
	}
}

func TestCollectiblePrice(t *testing.T) {
	cases := []struct {
		value     domain.Cents
		amount    string
		currency  string
		cryptoAmt string
	}{
		{0, "0", "", ""},
		{100, "1000000000", "TON", "1000000000"},                  // 1 GRAM
		{12345, "123450000000", "TON", "123450000000"},            // 123.45 GRAM
		{10_000_000, "100000000000000", "TON", "100000000000000"}, // cap
	}
	for _, c := range cases {
		amount, currency, cryptoAmt := collectiblePrice(network.Op{Value: c.value})
		if amount != c.amount || currency != c.currency || cryptoAmt != c.cryptoAmt {
			t.Errorf("collectiblePrice(%d) = %q %q %q, want %q %q %q", c.value, amount, currency, cryptoAmt, c.amount, c.currency, c.cryptoAmt)
		}
	}
}

// A retry must send byte-identical purchase metadata, otherwise the same
// command_id carries a different request and the server rejects it.
func TestGrantUsernameMetadataIsDeterministic(t *testing.T) {
	var bodies []map[string]any
	c, _ := fake(t, func(path string, body map[string]any) (int, any) {
		bodies = append(bodies, body)
		if path == "/v1/collectible-usernames/mint" {
			return 400, map[string]any{"status": "failed", "error": "username occupied", "code": "USERNAME_OCCUPIED"}
		}
		return 200, map[string]any{"status": "executed"}
	})
	op := network.Op{Key: "item:1", At: time.Unix(1700000000, 0), Value: 2500}
	for i := 0; i < 2; i++ {
		if err := c.GrantUsername(context.Background(), op, 42, "lunar"); err != nil {
			t.Fatal(err)
		}
	}
	if len(bodies) != 4 {
		t.Fatalf("requests = %d, want 4", len(bodies))
	}
	for i := 0; i < 4; i += 2 {
		if bodies[i]["purchase_date"] != float64(1700000000) {
			t.Fatalf("mint purchase_date = %v", bodies[i]["purchase_date"])
		}
		if bodies[i]["crypto_amount"] != "25000000000" || bodies[i]["amount"] != "25000000000" {
			t.Fatalf("mint amounts = %v %v", bodies[i]["amount"], bodies[i]["crypto_amount"])
		}
	}
	if bodies[0]["command_id"] != bodies[2]["command_id"] || bodies[1]["command_id"] != bodies[3]["command_id"] {
		t.Fatal("repeated delivery must reuse command ids")
	}
	if bodies[0]["command_id"] == bodies[1]["command_id"] {
		t.Fatal("mint and transfer must not share a command id")
	}
}

func TestDeleteChannelTreatsAlreadyDeletedAsSuccess(t *testing.T) {
	c, _ := fake(t, func(path string, _ map[string]any) (int, any) {
		if path != "/v1/channels/delete" {
			t.Fatalf("path = %s", path)
		}
		return http.StatusBadRequest, map[string]any{"message": "command failed", "error": "channel invalid"}
	})
	if err := c.DeleteChannel(context.Background(), network.Op{Key: "delete:275"}, 275); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
