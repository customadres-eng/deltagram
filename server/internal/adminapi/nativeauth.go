package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
)

// NativeAuthRequest is the server-to-server proof produced by the DeltaGram
// Android client. The client signs a canonical request string with the
// permanent MTProto auth key of the signed-in account.
type NativeAuthRequest struct {
	AuthKeyID  string `json:"auth_key_id"`
	Timestamp  int64  `json:"timestamp"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	BodySHA256 string `json:"body_sha256"`
	Nonce      string `json:"nonce"`
	Signature  string `json:"signature"`
}

// NativeAuthVerifier resolves a signed request to the Telegram user that owns
// the auth key. It must reject expired, replayed and malformed proofs.
type NativeAuthVerifier interface {
	VerifyNativeAuth(ctx context.Context, req NativeAuthRequest) (int64, error)
}

type nativeAuthResponse struct {
	UserID int64 `json:"user_id"`
}

func (s *Server) handleNativeVerify(w http.ResponseWriter, r *http.Request) {
	if s.native == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "native auth verifier is not configured"})
		return
	}
	var req NativeAuthRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	userID, err := s.native.VerifyNativeAuth(r.Context(), req)
	if err != nil || userID <= 0 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, nativeAuthResponse{UserID: userID})
}
