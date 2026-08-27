package deployment

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/buildinfo"
)

type Options struct {
	Listen            string
	StorageBackend    string
	TrustedProxies    []netip.Prefix
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

type Handler struct {
	admin   *adminauth.Handler
	options Options
	now     func() time.Time
}

func New(admin *adminauth.Handler, options Options) *Handler {
	return &Handler{admin: admin, options: options, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.admin.Authorize(r, false); !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	value := h.snapshot()
	if r.URL.Path == "/admin/v1/support-bundle" {
		w.Header().Set("Content-Disposition", `attachment; filename="trestle-support.json"`)
		value["generatedAt"] = h.now().UTC().Format(time.RFC3339)
	} else if r.URL.Path != "/admin/v1/deployment" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) snapshot() map[string]any {
	proxies := make([]string, len(h.options.TrustedProxies))
	for i, prefix := range h.options.TrustedProxies {
		proxies[i] = prefix.String()
	}
	return map[string]any{
		"version": buildinfo.Current(), "listen": h.options.Listen, "storageBackend": h.options.StorageBackend,
		"trustedProxies": proxies, "readHeaderTimeout": h.options.ReadHeaderTimeout.String(),
		"readTimeout": h.options.ReadTimeout.String(), "idleTimeout": h.options.IdleTimeout.String(), "maxHeaderBytes": h.options.MaxHeaderBytes,
		"secretsIncluded": false,
	}
}
