package gramsrv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/network/jsonhttp"
)

const Kind = "gramsrv"

func init() {
	network.Register(Kind, func(apiURL, token string) (network.Network, error) {
		return New(apiURL, token)
	})
}

// адаптер для приваток на gramsrv/telesrv через их Admin API
type Client struct {
	http  *jsonhttp.Client
	actor string
}

func New(apiURL, token string) (*Client, error) {
	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		return nil, fmt.Errorf("%w: api url must start with http(s)://", domain.ErrInvalid)
	}
	if token == "" {
		return nil, fmt.Errorf("%w: admin api token is empty", domain.ErrInvalid)
	}
	c := jsonhttp.New(apiURL)
	c.Headers = func([]byte) map[string]string {
		return map[string]string{"Authorization": "Bearer " + token}
	}
	return &Client{http: c, actor: "deltament"}, nil
}

// общие поля любой команды gramsrv
type meta struct {
	CommandID string `json:"command_id"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason"`
	DryRun    bool   `json:"dry_run"`
}

// metaAction derives one command id per (operation key, action). The Admin API
// keys idempotency by command_id globally, so two different requests that
// share a key (a channel delete followed by a username mint, for example) would
// collide with COMMAND_ID_CONFLICT. The action suffix keeps them distinct while
// staying stable across retries.
func (c *Client) metaAction(op network.Op, action string) meta {
	sum := sha256.Sum256([]byte(action + ":" + op.Key))
	reason := op.Reason
	if reason == "" {
		reason = "Deltament"
	}
	return meta{CommandID: "deltament-" + hex.EncodeToString(sum[:])[:40], Actor: c.actor, Reason: reason}
}

func (c *Client) meta(op network.Op) meta {
	return c.metaAction(op, "default")
}

// collectiblePrice renders the lot value as TON minor units. Amount and
// crypto_amount in the Admin API are nanotons, so one whole GRAM/TON is 1e9.
// A zero value omits the crypto fields: the validator rejects TON with amount 0.
func collectiblePrice(op network.Op) (amount, cryptoCurrency, cryptoAmount string) {
	nanotons := int64(op.Value) * 10_000_000
	if nanotons <= 0 {
		return "0", "", ""
	}
	value := strconv.FormatInt(nanotons, 10)
	return value, "TON", value
}

func purchaseUnix(op network.Op) int64 {
	if op.At.IsZero() {
		return 0
	}
	return op.At.Unix()
}

func (c *Client) Ping(ctx context.Context) error {
	return c.http.Do(ctx, "GET", "/v1/collectible-usernames?limit=1", nil, nil)
}

func (c *Client) ResolvePhone(ctx context.Context, phone string) (int64, error) {
	var out struct {
		Found  bool  `json:"found"`
		UserID int64 `json:"user_id"`
	}
	if err := c.http.Do(ctx, "POST", "/v1/accounts/resolve-by-phone", map[string]string{"phone": phone}, &out); err != nil {
		return 0, err
	}
	if !out.Found || out.UserID <= 0 {
		return 0, domain.ErrNotFound
	}
	return out.UserID, nil
}

// код уходит личным сообщением через рассылку на одного юзера
func (c *Client) SendCode(ctx context.Context, userID int64, text string) error {
	body := struct {
		meta
		Message    string  `json:"message"`
		TargetMode string  `json:"target_mode"`
		UserIDs    []int64 `json:"user_ids"`
	}{
		meta:       c.metaAction(network.Op{Key: fmt.Sprintf("code:%d:%d", userID, time.Now().UnixNano()), Reason: "Deltament login code"}, "send-code"),
		Message:    text,
		TargetMode: "selected",
		UserIDs:    []int64{userID},
	}
	return c.http.Do(ctx, "POST", "/v1/broadcasts/create", body, nil)
}

func (c *Client) GrantUsername(ctx context.Context, op network.Op, userID int64, username string) error {
	m := c.metaAction(op, "grant-username-mint")
	amount, cryptoCurrency, cryptoAmount := collectiblePrice(op)
	mint := struct {
		meta
		Username       string `json:"username"`
		OwnerUserID    string `json:"owner_user_id"`
		Currency       string `json:"currency"`
		Amount         string `json:"amount"`
		CryptoCurrency string `json:"crypto_currency,omitempty"`
		CryptoAmount   string `json:"crypto_amount,omitempty"`
		PurchaseDate   int64  `json:"purchase_date,omitempty"`
	}{m, username, strconv.FormatInt(userID, 10), "TON", amount, cryptoCurrency, cryptoAmount, purchaseUnix(op)}
	err := c.http.Do(ctx, "POST", "/v1/collectible-usernames/mint", mint, nil)
	if !occupied(err) {
		return err
	}
	// юзернейм уже есть в хранилище сервера (например, в vault после эскроу),
	// просто передаём его победителю
	transfer := struct {
		meta
		Username string `json:"username"`
		ToUserID string `json:"to_user_id"`
	}{c.metaAction(op, "grant-username-transfer"), username, strconv.FormatInt(userID, 10)}
	return c.http.Do(ctx, "POST", "/v1/collectible-usernames/transfer", transfer, nil)
}

func (c *Client) GrantNumber(ctx context.Context, op network.Op, userID int64, phone string) error {
	m := c.metaAction(op, "grant-number-mint")
	digits := onlyDigits(phone)
	amount, cryptoCurrency, cryptoAmount := collectiblePrice(op)
	mint := struct {
		meta
		Phone          string `json:"phone"`
		Tier           string `json:"tier"`
		OwnerUserID    string `json:"owner_user_id"`
		Currency       string `json:"currency"`
		Amount         string `json:"amount"`
		CryptoCurrency string `json:"crypto_currency,omitempty"`
		CryptoAmount   string `json:"crypto_amount,omitempty"`
		PurchaseDate   int64  `json:"purchase_date,omitempty"`
	}{m, digits, "standard", strconv.FormatInt(userID, 10), "TON", amount, cryptoCurrency, cryptoAmount, purchaseUnix(op)}
	err := c.http.Do(ctx, "POST", "/v1/collectible-phones/mint", mint, nil)
	if !occupied(err) {
		return err
	}
	transfer := struct {
		meta
		Phone    string `json:"phone"`
		ToUserID string `json:"to_user_id"`
	}{c.metaAction(op, "grant-number-transfer"), digits, strconv.FormatInt(userID, 10)}
	return c.http.Do(ctx, "POST", "/v1/collectible-phones/transfer", transfer, nil)
}

func (c *Client) GrantGift(ctx context.Context, op network.Op, userID int64, giftID int64) error {
	body := struct {
		meta
		UserID  int64  `json:"user_id"`
		GiftID  int64  `json:"gift_id"`
		Upgrade bool   `json:"upgrade"`
		Message string `json:"message"`
	}{c.metaAction(op, "grant-gift"), userID, giftID, true, "Покупка на Deltament"}
	return c.http.Do(ctx, "POST", "/v1/gifts/give", body, nil)
}

func (c *Client) GrantStars(ctx context.Context, op network.Op, userID int64, amount int64) error {
	body := struct {
		meta
		UserID int64 `json:"user_id"`
		Amount int64 `json:"amount"`
	}{c.metaAction(op, "grant-stars"), userID, amount}
	return c.http.Do(ctx, "POST", "/v1/accounts/grant-stars", body, nil)
}

func (c *Client) DebitStars(ctx context.Context, op network.Op, userID int64, amount int64) error {
	body := struct {
		meta
		UserID int64 `json:"user_id"`
		Amount int64 `json:"amount"`
	}{c.metaAction(op, "debit-stars"), userID, amount}
	err := c.http.Do(ctx, "POST", "/v1/accounts/debit-stars", body, nil)
	var re *jsonhttp.Error
	if errors.As(err, &re) && re.Status == 400 && strings.Contains(strings.ToLower(re.Text), "insufficient") {
		return domain.ErrNoFunds
	}
	return err
}

// OwnedChannels returns channel/supergroup assets created by a user.
func (c *Client) OwnedChannels(ctx context.Context, userID int64) ([]network.ChannelAsset, error) {
	var out struct {
		Channels []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Username string `json:"username"`
		} `json:"channels"`
	}
	path := "/v1/channels/created?user_id=" + strconv.FormatInt(userID, 10) + "&limit=200"
	if err := c.http.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	assets := make([]network.ChannelAsset, 0, len(out.Channels))
	for _, item := range out.Channels {
		id, err := strconv.ParseInt(item.ID, 10, 64)
		if err != nil || id <= 0 || item.Username == "" {
			continue
		}
		assets = append(assets, network.ChannelAsset{ID: id, Title: item.Title, Username: item.Username})
	}
	return assets, nil
}

// OwnedUsernames returns collectible usernames currently held by a user.
func (c *Client) OwnedUsernames(ctx context.Context, userID int64) ([]network.UsernameAsset, error) {
	var out struct {
		Assets []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"assets"`
	}
	path := "/v1/collectible-usernames?status=owned&owner_user_id=" + strconv.FormatInt(userID, 10) + "&limit=200"
	if err := c.http.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	assets := make([]network.UsernameAsset, 0, len(out.Assets))
	for _, item := range out.Assets {
		id, err := strconv.ParseInt(item.ID, 10, 64)
		if err != nil || id <= 0 || item.Username == "" || (item.Status != "" && item.Status != "owned") {
			continue
		}
		assets = append(assets, network.UsernameAsset{ID: id, Username: item.Username})
	}
	return assets, nil
}

// DeleteChannel soft-deletes a channel through the admin API. A missing channel
// counts as success: settlement may retry after the delete already committed.
func (c *Client) DeleteChannel(ctx context.Context, op network.Op, channelID int64) error {
	body := struct {
		meta
		ChannelID int64 `json:"channel_id"`
	}{c.metaAction(op, "delete-channel"), channelID}
	err := c.http.Do(ctx, "POST", "/v1/channels/delete", body, nil)
	if missing(err) {
		return nil
	}
	return err
}

// TransferUsername moves an existing collectible username to a user.
func (c *Client) TransferUsername(ctx context.Context, op network.Op, username string, userID int64) error {
	body := struct {
		meta
		Username string `json:"username"`
		ToUserID string `json:"to_user_id"`
	}{c.metaAction(op, "transfer-username"), username, strconv.FormatInt(userID, 10)}
	err := c.http.Do(ctx, "POST", "/v1/collectible-usernames/transfer", body, nil)
	if missing(err) {
		return domain.ErrNotFound
	}
	return err
}

// MintUsernameToVault escrows a channel username: the channel must already be
// deleted, so this mints the freed name into the operator vault with the sale
// price attached. A retry after a committed mint gets OCCUPIED, which means the
// name is already in the vault and is safe to treat as success.
func (c *Client) MintUsernameToVault(ctx context.Context, op network.Op, username string) error {
	amount, cryptoCurrency, cryptoAmount := collectiblePrice(op)
	body := struct {
		meta
		Username       string `json:"username"`
		Currency       string `json:"currency"`
		Amount         string `json:"amount"`
		CryptoCurrency string `json:"crypto_currency,omitempty"`
		CryptoAmount   string `json:"crypto_amount,omitempty"`
		PurchaseDate   int64  `json:"purchase_date,omitempty"`
	}{c.metaAction(op, "escrow-mint-username"), username, "TON", amount, cryptoCurrency, cryptoAmount, purchaseUnix(op)}
	err := c.http.Do(ctx, "POST", "/v1/collectible-usernames/mint", body, nil)
	if occupied(err) {
		return nil
	}
	return err
}

// RevokeUsername returns a seller's collectible username to the vault. Revoking
// an asset that is already unowned is a successful retry, not a failure.
func (c *Client) RevokeUsername(ctx context.Context, op network.Op, username string) error {
	body := struct {
		meta
		Username string `json:"username"`
		Burn     bool   `json:"burn"`
	}{c.metaAction(op, "escrow-revoke-username"), username, false}
	err := c.http.Do(ctx, "POST", "/v1/collectible-usernames/revoke", body, nil)
	if missing(err) || notOwned(err) {
		return nil
	}
	return err
}

func notOwned(err error) bool {
	var re *jsonhttp.Error
	if !errors.As(err, &re) {
		return false
	}
	s := strings.ToLower(re.Code + " " + re.Text)
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	return strings.Contains(s, "not owned") || strings.Contains(s, "vault")
}

func missing(err error) bool {
	var re *jsonhttp.Error
	if !errors.As(err, &re) {
		return false
	}
	if re.Status == 404 {
		return true
	}
	s := strings.ToLower(re.Code + " " + re.Text)
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	return strings.Contains(s, "not found") || strings.Contains(s, "channel invalid") ||
		strings.Contains(s, "already deleted") || strings.Contains(s, "channel not found")
}

// сервер сказал что такой ассет уже есть
func occupied(err error) bool {
	var re *jsonhttp.Error
	if !errors.As(err, &re) {
		return false
	}
	s := strings.ToUpper(re.Code + " " + re.Text)
	return strings.Contains(s, "OCCUPIED") || strings.Contains(s, "EXIST") || strings.Contains(s, "ALREADY")
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
