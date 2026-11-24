package httpapi

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"

	"telesrv/cmd/deltament/internal/domain"
)

const maxBannerBytes = 4 << 20

// публичный реестр баннеров: клиенты по нему узнают, у кого есть картинка

func (a *API) getBannerRegistry(w http.ResponseWriter, r *http.Request) {
	tokens, err := a.Banners.BannerTokens(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"version": 1, "banners": tokens})
}

func (a *API) getBanner(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || userID <= 0 {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	b, err := a.Banners.UserBanner(r.Context(), userID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.Header().Set("Content-Type", b.MIME)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+b.Token+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b.Bytes)
}

func (a *API) myBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	token, err := a.Banners.BannerToken(r.Context(), u.ServerUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, 200, map[string]any{"token": "", "url": ""})
			return
		}
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "url": bannerURL(u.ServerUserID, token)})
}

func bannerURL(userID int64, token string) string {
	if token == "" {
		return ""
	}
	return "/api/banners/" + strconv.FormatInt(userID, 10) + "?v=" + token
}

// normalizeBannerCover crops a banner to the 3:1 profile cover ratio and
// re-encodes it as JPEG. The client draws a banner by fitting its width, so a
// tall source would otherwise be zoomed into an extreme close-up.
func normalizeBannerCover(body []byte) ([]byte, string, error) {
	src, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	cropW, cropH := width, width/3
	if cropH < 1 {
		cropH = 1
	}
	if cropH > height {
		cropH = height
		cropW = height * 3
		if cropW > width {
			cropW = width
		}
	}
	cropX := bounds.Min.X + (width-cropW)/2
	cropY := bounds.Min.Y + (height-cropH)/2
	dst := image.NewRGBA(image.Rect(0, 0, cropW, cropH))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(cropX, cropY), draw.Src)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, "", err
	}
	return out.Bytes(), "image/jpeg", nil
}

func (a *API) uploadBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBannerBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	if len(body) < 64 {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(body))
	if cfgErr != nil || cfg.Width < 320 || cfg.Height < 100 || cfg.Width > 6000 || cfg.Height > 6000 {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	normalized, mime, err := normalizeBannerCover(body)
	if err != nil {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	token, err := a.Banners.SetUserBanner(r.Context(), u.ServerUserID, mime, normalized)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "url": bannerURL(u.ServerUserID, token)})
}

func (a *API) deleteBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if err := a.Banners.ClearUserBanner(r.Context(), u.ServerUserID); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
