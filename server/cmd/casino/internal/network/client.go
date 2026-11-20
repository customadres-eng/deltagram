// Package network adapts the main telesrv server's Admin API for the casino.
// Stars move through the same gateway DeltaMent uses, with an idempotent
// command id so a retry can never double-mint or double-debit a player.
package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal JSON client for the telesrv Admin API.
type Client struct {
	base  string
	token string
	actor string
	http  *http.Client
}

// New builds a client against the Admin API. token may be empty to run in a
// "read-only" mode where star movements are disabled.
func New(apiURL, token, actor string) (*Client, error) {
	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		return nil, fmt.Errorf("admin api url must start with http(s)://")
	}
	return &Client{
		base:  strings.TrimRight(apiURL, "/"),
		token: strings.TrimSpace(token),
		actor: actor,
		http:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Enabled reports whether star movements are configured.
func (c *Client) Enabled() bool { return c != nil && c.token != "" }

// RemoteError is a non-2xx response from the Admin API.
type RemoteError struct {
	Status int
	Code   string
	Text   string
}

func (e *RemoteError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("remote %d %s: %s", e.Status, e.Code, e.Text)
	}
	return fmt.Sprintf("remote %d: %s", e.Status, e.Text)
}

type meta struct {
	CommandID string `json:"command_id"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason"`
	DryRun    bool   `json:"dry_run"`
}

// GrantStars credits a player's star balance on the main server.
func (c *Client) GrantStars(ctx context.Context, commandKey, reason string, userID, amount int64) error {
	return c.starCommand(ctx, "grant-stars", commandKey, reason, userID, amount)
}

// DebitStars debits a player's star balance on the main server. It maps the
// Admin API's insufficient-funds error to ErrInsufficient for the caller.
func (c *Client) DebitStars(ctx context.Context, commandKey, reason string, userID, amount int64) error {
	err := c.starCommand(ctx, "debit-stars", commandKey, reason, userID, amount)
	var re *RemoteError
	if errors.As(err, &re) && re.Status == 400 && strings.Contains(strings.ToLower(re.Text), "insufficient") {
		return ErrInsufficient
	}
	return err
}

func (c *Client) starCommand(ctx context.Context, action, commandKey, reason string, userID, amount int64) error {
	if !c.Enabled() {
		return ErrStarsDisabled
	}
	sum := sha256.Sum256([]byte(action + ":" + commandKey))
	if reason == "" {
		reason = "Casino"
	}
	body := struct {
		meta
		UserID int64 `json:"user_id"`
		Amount int64 `json:"amount"`
	}{
		meta:   meta{CommandID: c.actor + "-" + hex.EncodeToString(sum[:])[:40], Actor: c.actor, Reason: reason},
		UserID: userID, Amount: amount,
	}
	return c.do(ctx, http.MethodPost, "/v1/accounts/"+action, body, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return parseRemoteError(res.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func parseRemoteError(status int, raw []byte) error {
	e := &RemoteError{Status: status, Text: strings.TrimSpace(string(raw))}
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(raw, &body) == nil {
		e.Code = body.Code
		if body.Error != "" {
			e.Text = body.Error
		} else if body.Message != "" {
			e.Text = body.Message
		}
	}
	if len(e.Text) > 300 {
		e.Text = e.Text[:300]
	}
	return e
}
