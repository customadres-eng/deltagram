package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/httpapi"
)

type nativeAuthHTTP struct {
	url   string
	token string
	client *http.Client
}

func newNativeAuthHTTP(url, token string) *nativeAuthHTTP {
	return &nativeAuthHTTP{
		url:    strings.TrimRight(strings.TrimSpace(url), "/"),
		token:  strings.TrimSpace(token),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (v *nativeAuthHTTP) VerifyNativeAuth(ctx context.Context, req httpapi.NativeAuthRequest) (int64, error) {
	if v == nil || v.url == "" || v.token == "" {
		return 0, fmt.Errorf("native auth verifier is not configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url+"/v1/native/verify", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+v.token)
	resp, err := v.client.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("native auth rejected: HTTP %d", resp.StatusCode)
	}
	var out struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out); err != nil {
		return 0, err
	}
	if out.UserID <= 0 {
		return 0, fmt.Errorf("native auth returned empty user")
	}
	return out.UserID, nil
}
