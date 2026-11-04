package identity

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

const defaultCookieName = "famiy_session"

// HandlerConfig is the HTTP-facing part of the configuration.
type HandlerConfig struct {
	CookieName     string         // zero value means "famiy_session"
	CookieSecure   bool           // Secure attribute; false only for plain-HTTP local development
	TrustedProxies []netip.Prefix // peers whose X-Forwarded-For is believed
}

type Handler struct {
	svc *Service
	cfg HandlerConfig
}

func NewHandler(svc *Service, cfg HandlerConfig) *Handler {
	if cfg.CookieName == "" {
		cfg.CookieName = defaultCookieName
	}
	return &Handler{svc: svc, cfg: cfg}
}

// Routes registers the public auth routes; /auth/me needs a session.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/logout", h.logout)
	r.With(h.RequireAuth).Get("/auth/me", h.me)
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u User) userResponse {
	return userResponse(u)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, sess, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.setCookie(w, sess)
	httpx.JSON(w, r, http.StatusCreated, toUserResponse(u))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, sess, err := h.svc.Login(r.Context(), req.Email, req.Password, ClientAddr(r, h.cfg.TrustedProxies))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.setCookie(w, sess)
	httpx.JSON(w, r, http.StatusOK, toUserResponse(u))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(h.cfg.CookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context()) // RequireAuth guarantees it
	httpx.JSON(w, r, http.StatusOK, toUserResponse(u))
}

type ctxKey struct{}

// RequireAuth rejects requests without a valid session and otherwise puts the
// user in the request context.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(h.cfg.CookieName)
		if err != nil {
			httpx.Error(w, r, ErrUnauthenticated)
			return
		}
		u, err := h.svc.Authenticate(r.Context(), c.Value)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// UserFromContext returns the authenticated user placed by RequireAuth.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

func (h *Handler) setCookie(w http.ResponseWriter, s Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CookieName,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.ExpiresAt,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClientAddr returns the address used to key the login limiter. X-Forwarded-For
// is honoured only when the direct peer is a trusted proxy; then the right-most
// entry that is not itself a trusted proxy is the client.
func ClientAddr(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !within(peer, trusted) {
		return host
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		if !within(a, trusted) {
			return a.String()
		}
	}
	return host
}

func within(a netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
