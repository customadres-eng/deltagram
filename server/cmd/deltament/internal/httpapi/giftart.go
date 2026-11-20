package httpapi

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"telesrv/cmd/deltament/internal/domain"
)

// Gift art proxy.
//
// The web client used to hotlink https://nft.fragment.com directly. On mobile
// networks that third-party hop is slow, sometimes blocked, and impossible to
// cache with our own headers. Serving the same files from our origin keeps the
// page fast and makes gift images load in every client (including the Android
// and Telegram in-app browsers) because it is a same-origin request.

var giftArtName = regexp.MustCompile(`^[a-z0-9]+-[0-9]+\.webp$`)

const (
	maxGiftArtBytes = 48 << 20 // memory budget for cached art
	maxGiftArtFile  = 4 << 20  // a single card is ~50 KB
	giftArtTTL      = 24 * time.Hour
)

type giftArtEntry struct {
	data []byte
	at   time.Time
}

var (
	giftArtMu    sync.Mutex
	giftArtCache = map[string]giftArtEntry{}
	giftArtSize  int64
	giftArtHTTP  = &http.Client{Timeout: 8 * time.Second}
)

func giftArtLookup(name string) ([]byte, bool) {
	giftArtMu.Lock()
	defer giftArtMu.Unlock()
	e, ok := giftArtCache[name]
	if !ok {
		return nil, false
	}
	if time.Since(e.at) > giftArtTTL {
		delete(giftArtCache, name)
		giftArtSize -= int64(len(e.data))
		return nil, false
	}
	return e.data, true
}

func giftArtStore(name string, data []byte) {
	giftArtMu.Lock()
	defer giftArtMu.Unlock()
	if old, ok := giftArtCache[name]; ok {
		giftArtSize -= int64(len(old.data))
	}
	// Drop the cache wholesale when it grows past the budget. Gift art is
	// immutable, so a cold refill is just one extra origin request.
	if giftArtSize+int64(len(data)) > maxGiftArtBytes {
		giftArtCache = map[string]giftArtEntry{}
		giftArtSize = 0
	}
	giftArtCache[name] = giftArtEntry{data: data, at: time.Now()}
	giftArtSize += int64(len(data))
}

func (a *API) getGiftArt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !giftArtName.MatchString(name) {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	if data, ok := giftArtLookup(name); ok {
		writeGiftArt(w, data)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://nft.fragment.com/gift/"+name, nil)
	if err != nil {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36")
	req.Header.Set("Accept", "image/webp,image/*;q=0.9,*/*;q=0.8")
	resp, err := giftArtHTTP.Do(req)
	if err != nil {
		a.Log.Warn("gift art fetch failed", "name", name, "err", err)
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGiftArtFile))
	if err != nil || len(data) == 0 {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	giftArtStore(name, data)
	writeGiftArt(w, data)
}

// Gift animation cache.
//
// Catalog/model/pattern animations are immutable JSON blobs stored in the
// database (100 KB - 700 KB each). Reading them from Postgres on every card
// render would be wasteful, so they are cached in memory like the webp art.
const (
	maxGiftAnimBytes = 96 << 20
	giftAnimTTL      = 24 * time.Hour
)

type giftAnimEntry struct {
	data []byte
	at   time.Time
}

var (
	giftAnimMu    sync.Mutex
	giftAnimCache = map[string]giftAnimEntry{}
	giftAnimSize  int64
)

func giftAnimLookup(key string) ([]byte, bool) {
	giftAnimMu.Lock()
	defer giftAnimMu.Unlock()
	e, ok := giftAnimCache[key]
	if !ok {
		return nil, false
	}
	if time.Since(e.at) > giftAnimTTL {
		delete(giftAnimCache, key)
		giftAnimSize -= int64(len(e.data))
		return nil, false
	}
	return e.data, true
}

func giftAnimStore(key string, data []byte) {
	giftAnimMu.Lock()
	defer giftAnimMu.Unlock()
	if old, ok := giftAnimCache[key]; ok {
		giftAnimSize -= int64(len(old.data))
	}
	if giftAnimSize+int64(len(data)) > maxGiftAnimBytes {
		giftAnimCache = map[string]giftAnimEntry{}
		giftAnimSize = 0
	}
	giftAnimCache[key] = giftAnimEntry{data: data, at: time.Now()}
	giftAnimSize += int64(len(data))
}

func (a *API) serveGiftAnimation(w http.ResponseWriter, r *http.Request, kind string, load func(context.Context, int64) ([]byte, error)) {
	id, ok := pathID(r, "id")
	if !ok || a.GiftArt == nil {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	key := kind + ":" + strconv.FormatInt(id, 10)
	if data, ok := giftAnimLookup(key); ok {
		writeGiftAnimation(w, data)
		return
	}
	data, err := load(r.Context(), id)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	giftAnimStore(key, data)
	writeGiftAnimation(w, data)
}

func (a *API) getGiftCatalogAnimation(w http.ResponseWriter, r *http.Request) {
	a.serveGiftAnimation(w, r, "catalog", a.GiftArt.GiftCatalogAnimation)
}

func (a *API) getGiftModelAnimation(w http.ResponseWriter, r *http.Request) {
	a.serveGiftAnimation(w, r, "model", a.GiftArt.GiftModelAnimation)
}

func (a *API) getGiftPatternAnimation(w http.ResponseWriter, r *http.Request) {
	a.serveGiftAnimation(w, r, "pattern", a.GiftArt.GiftPatternAnimation)
}

func writeGiftAnimation(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func writeGiftArt(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
