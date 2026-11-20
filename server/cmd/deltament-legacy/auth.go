package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
func loadLoginKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		path = "/var/lib/telesrv/telegram-login/signing-rs256.pem"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("invalid login key")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if key, ok := k.(*rsa.PrivateKey); ok {
			return key, nil
		}
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
func loadSessionKey(path string) ([]byte, error) {
	if path == "" {
		path = "/var/lib/telesrv/telegram-login/session-key"
	}
	b, err := os.ReadFile(path)
	if err == nil && len(b) >= 32 {
		return b, nil
	}
	b = make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		return nil, err
	}
	return b, nil
}
func (a *app) apiOIDC(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDToken string `json:"id_token"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil || body.IDToken == "" {
		http.Error(w, "{\"error\":\"INVALID_ID_TOKEN\"}", 400)
		return
	}
	tok, err := jwt.Parse([]byte(body.IDToken), jwt.WithKey(jwa.RS256(), &a.loginKey.PublicKey), jwt.WithIssuer(a.loginIssuer), jwt.WithAudience("8933143600"))
	if err != nil {
		http.Error(w, "{\"error\":\"INVALID_ID_TOKEN\"}", 401)
		return
	}
	sub, ok := tok.Subject()
	if !ok {
		http.Error(w, "{\"error\":\"INVALID_ID_TOKEN\"}", 401)
		return
	}
	uid, err := strconv.ParseInt(sub, 10, 64)
	if err != nil || uid <= 0 {
		http.Error(w, "{\"error\":\"INVALID_ID_TOKEN\"}", 401)
		return
	}
	a.setSession(w, uid)
	writeJSON(w, map[string]any{"ok": true})
}
func (a *app) setSession(w http.ResponseWriter, uid int64) {
	raw := fmt.Sprintf("%d.%d", uid, time.Now().Add(30*24*time.Hour).Unix())
	mac := hmac.New(sha256.New, a.sessionKey)
	mac.Write([]byte(raw))
	http.SetCookie(w, &http.Cookie{Name: "deltament_session", Value: raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 2592000})
}
func (a *app) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "deltament_session", Value: "", Path: "/", HttpOnly: true, Secure: true, MaxAge: -1})
}
func (a *app) sessionUser(r *http.Request) (int64, bool) {
	c, err := r.Cookie("deltament_session")
	if err != nil {
		return 0, false
	}
	p := strings.Split(c.Value, ".")
	if len(p) != 3 {
		return 0, false
	}
	exp, err := strconv.ParseInt(p[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	mac := hmac.New(sha256.New, a.sessionKey)
	mac.Write([]byte(p[0] + "." + p[1]))
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, false
	}
	uid, err := strconv.ParseInt(p[0], 10, 64)
	return uid, err == nil && uid > 0
}
