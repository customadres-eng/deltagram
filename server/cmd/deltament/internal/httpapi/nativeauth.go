package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"telesrv/cmd/deltament/internal/domain"
)

// NativeAuthRequest is the proof produced by the DeltaGram Android client.
type NativeAuthRequest struct {
	AuthKeyID  string `json:"auth_key_id"`
	Timestamp  int64  `json:"timestamp"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	BodySHA256 string `json:"body_sha256"`
	Nonce      string `json:"nonce"`
	Signature  string `json:"signature"`
}

type NativeAuthVerifier interface {
	VerifyNativeAuth(ctx context.Context, req NativeAuthRequest) (int64, error)
}

type nativeUserKey struct{}

func nativeUser(r *http.Request) int64 {
	id, _ := r.Context().Value(nativeUserKey{}).(int64)
	return id
}

// nativeAuth verifies the MTProto-auth-key HMAC signature sent by the native
// DeltaGram client. The user id comes only from the verified telesrv auth key;
// X-DeltaGram-User and query parameters are never trusted.
func (a *API) nativeAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.Native == nil {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "DeltaGram ") {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(header, "DeltaGram "), ":", 4)
		if len(parts) != 4 {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		authKeyID := parts[0]
		timestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		nonce := parts[2]
		signature := parts[3]

		var body []byte
		if r.Body != nil {
			body, err = io.ReadAll(io.LimitReader(r.Body, 16<<20))
			if err != nil {
				writeErr(w, a.Log, domain.ErrUnauthorized)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		sum := sha256.Sum256(body)
		req := NativeAuthRequest{
			AuthKeyID:  authKeyID,
			Timestamp:  timestamp,
			Method:     r.Method,
			Path:       r.URL.RequestURI(),
			BodySHA256: hex.EncodeToString(sum[:]),
			Nonce:      nonce,
			Signature:  signature,
		}
		userID, err := a.Native.VerifyNativeAuth(r.Context(), req)
		if err != nil || userID <= 0 {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), nativeUserKey{}, userID)))
	}
}
