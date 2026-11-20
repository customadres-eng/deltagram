package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"telesrv/internal/adminapi"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

type nativeAuthKeyStore interface {
	Get(ctx context.Context, id [8]byte) (store.AuthKeyData, bool, error)
}

type nativeAuthorizationStore interface {
	ByAuthKey(ctx context.Context, authKeyID [8]byte) (domain.Authorization, bool, error)
}

type nativeAuthVerifier struct {
	keys  nativeAuthKeyStore
	authz nativeAuthorizationStore
	now   func() time.Time

	mu   sync.Mutex
	seen map[string]int64
}

func newNativeAuthVerifier(keys nativeAuthKeyStore, authz nativeAuthorizationStore) *nativeAuthVerifier {
	return &nativeAuthVerifier{
		keys:  keys,
		authz: authz,
		now:   time.Now,
		seen:  make(map[string]int64),
	}
}

var errNativeAuth = errors.New("native auth rejected")

func (v *nativeAuthVerifier) VerifyNativeAuth(ctx context.Context, req adminapi.NativeAuthRequest) (int64, error) {
	if v == nil || v.keys == nil || v.authz == nil {
		return 0, errNativeAuth
	}
	now := v.now()
	if req.Timestamp < now.Unix()-120 || req.Timestamp > now.Unix()+120 {
		return 0, errNativeAuth
	}
	if req.Method == "" || req.Path == "" || len(req.Nonce) < 16 || len(req.Nonce) > 64 {
		return 0, errNativeAuth
	}
	if len(req.BodySHA256) != 64 || len(req.Signature) != 64 {
		return 0, errNativeAuth
	}
	rawID, err := strconv.ParseUint(strings.TrimPrefix(req.AuthKeyID, "0x"), 16, 64)
	if err != nil {
		return 0, errNativeAuth
	}
	var id [8]byte
	binary.LittleEndian.PutUint64(id[:], rawID)

	auth, found, err := v.authz.ByAuthKey(ctx, id)
	if err != nil || !found || auth.UserID <= 0 {
		return 0, errNativeAuth
	}
	key, found, err := v.keys.Get(ctx, id)
	if err != nil || !found {
		return 0, errNativeAuth
	}

	canonical := req.Method + "\n" + req.Path + "\n" + strconv.FormatInt(req.Timestamp, 10) + "\n" + req.BodySHA256 + "\n" + req.Nonce
	mac := hmac.New(sha256.New, key.Value[:])
	_, _ = mac.Write([]byte(canonical))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(req.Signature)
	if err != nil || !hmac.Equal(want, got) {
		return 0, errNativeAuth
	}

	nonceKey := req.AuthKeyID + ":" + req.Nonce
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, exp := range v.seen {
		if exp < now.Unix() {
			delete(v.seen, k)
		}
	}
	if _, replay := v.seen[nonceKey]; replay {
		return 0, errNativeAuth
	}
	v.seen[nonceKey] = now.Unix() + 120
	return auth.UserID, nil
}
