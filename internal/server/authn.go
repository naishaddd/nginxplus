package server

// Authentication middleware and /api/auth/* endpoints.
//
// First run: no admin exists → every page redirects to /setup.html, where the
// operator enters the one-time setup code (printed in the container logs, or
// NGINXPP_SETUP_CODE) and chooses a username + password. For unattended
// deploys, NGINXPP_ADMIN_USER + NGINXPP_ADMIN_PASSWORD create the account at
// startup instead.
//
// Afterwards: signed HttpOnly session cookie (7 days). NGINXPP_API_TOKEN, if
// set, is also accepted as "Authorization: Bearer" for scripts and CI.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/naishaddd/nginxplus/internal/auth"
	"github.com/naishaddd/nginxplus/internal/config"
)

const sessionCookie = "nginxpp_session"

var (
	authStore   *auth.Store
	loginLimit  = auth.NewLimiter()
	setupCode   string
	apiToken    = config.Env("NGINXPP_API_TOKEN", config.Env("NGINXPP_TOKEN", "")) // NGINXPP_TOKEN: legacy name
	publicPaths = map[string]bool{
		"/login.html": true, "/setup.html": true, "/auth.js": true, "/design.css": true,
		"/app.css": true, "/logo.svg": true, "/favicon.svg": true,
		"/api/auth/login": true, "/api/auth/setup": true, "/api/auth/state": true,
	}
)

// InitAuth opens the account store. Call once at startup.
func InitAuth() {
	var err error
	authStore, err = auth.Open(filepath.Join(config.Env("NGINXPP_DATA_DIR", "/data")))
	if err != nil {
		log.Fatalf("auth store: %v", err)
	}
	if authStore.IsSetUp() {
		log.Printf("admin account: %s", authStore.User())
		return
	}
	if u, p := config.Env("NGINXPP_ADMIN_USER", ""), config.Env("NGINXPP_ADMIN_PASSWORD", ""); u != "" && p != "" {
		if err := authStore.Setup(u, p); err != nil {
			log.Fatalf("NGINXPP_ADMIN_USER/PASSWORD rejected: %v", err)
		}
		log.Printf("admin account %q created from environment. Remove NGINXPP_ADMIN_PASSWORD from your config now.", u)
		return
	}
	setupCode = config.Env("NGINXPP_SETUP_CODE", "")
	if setupCode == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		h := strings.ToUpper(hex.EncodeToString(b)) // 48 random bits
		setupCode = h[0:4] + "-" + h[4:8] + "-" + h[8:12]
	}
	log.Printf("────────────────────────────────────────────────────────")
	log.Printf(" First run: open the dashboard and create the admin account.")
	log.Printf(" Setup code: %s", setupCode)
	log.Printf("────────────────────────────────────────────────────────")
}

func isPublic(p string) bool {
	return publicPaths[p] || strings.HasPrefix(p, "/vendor/fonts/") || strings.HasPrefix(p, "/vendor/fontawesome/")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// sameOriginWrite rejects cross-site state-changing requests (defence in depth on top of SameSite cookies).
func sameOriginWrite(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client (curl with bearer token)
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func setSession(w http.ResponseWriter, r *http.Request) {
	v, exp := authStore.NewSession(time.Now())
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: v, Path: "/", Expires: exp,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode})
}

func currentUser(r *http.Request) (string, bool) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if u, ok := authStore.CheckSession(c.Value, time.Now()); ok {
			return u, true
		}
	}
	if h := r.Header.Get("Authorization"); apiToken != "" && strings.HasPrefix(h, "Bearer ") &&
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h, "Bearer ")), []byte(apiToken)) == 1 {
		return "api-token", true
	}
	return "", false
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOriginWrite(r) {
			jsonErr(w, http.StatusForbidden, "cross-origin request blocked")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			handleAuthAPI(w, r)
			return
		}
		if isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		api := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/")
		if !authStore.IsSetUp() {
			if api {
				jsonErr(w, http.StatusServiceUnavailable, "setup required")
			} else {
				http.Redirect(w, r, "/setup.html", http.StatusFound)
			}
			return
		}
		if _, ok := currentUser(r); !ok {
			if api {
				jsonErr(w, http.StatusUnauthorized, "unauthorized")
			} else {
				http.Redirect(w, r, "/login.html?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			}
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func handleAuthAPI(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Password, SetupCode, Current, New string }
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && r.URL.Path != "/api/auth/logout" {
			jsonErr(w, 400, "invalid JSON")
			return
		}
	}
	ip, now := clientIP(r), time.Now()

	switch r.URL.Path {
	case "/api/auth/state":
		u, ok := currentUser(r)
		writeJSON(w, 200, map[string]interface{}{"setup": authStore.IsSetUp(), "authenticated": ok, "user": u})

	case "/api/auth/setup":
		if r.Method != http.MethodPost {
			jsonErr(w, 405, "POST required")
			return
		}
		if authStore.IsSetUp() {
			jsonErr(w, 409, "admin account already exists")
			return
		}
		if b, wait := loginLimit.Blocked(ip, now); b {
			jsonErr(w, 429, "too many attempts, try again in "+wait.Round(time.Minute).String())
			return
		}
		if setupCode == "" || subtle.ConstantTimeCompare([]byte(normCode(body.SetupCode)), []byte(normCode(setupCode))) != 1 {
			loginLimit.Fail(ip, now)
			jsonErr(w, 403, "wrong setup code. Check the container logs (docker logs nginxpp)")
			return
		}
		if err := authStore.Setup(body.Username, body.Password); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		log.Printf("admin account %q created from %s", authStore.User(), ip)
		setupCode = ""
		setSession(w, r)
		writeJSON(w, 200, map[string]string{"status": "ok"})

	case "/api/auth/login":
		if r.Method != http.MethodPost {
			jsonErr(w, 405, "POST required")
			return
		}
		if b, wait := loginLimit.Blocked(ip, now); b {
			jsonErr(w, 429, "too many failed attempts, try again in "+wait.Round(time.Minute).String())
			return
		}
		if err := authStore.Verify(body.Username, body.Password); err != nil {
			loginLimit.Fail(ip, now)
			log.Printf("failed login for %q from %s", body.Username, ip)
			jsonErr(w, 401, "invalid username or password")
			return
		}
		loginLimit.Reset(ip)
		setSession(w, r)
		writeJSON(w, 200, map[string]string{"status": "ok"})

	case "/api/auth/logout":
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, 200, map[string]string{"status": "ok"})

	case "/api/auth/password":
		if _, ok := currentUser(r); !ok {
			jsonErr(w, 401, "unauthorized")
			return
		}
		if r.Method != http.MethodPost {
			jsonErr(w, 405, "POST required")
			return
		}
		if err := authStore.ChangePassword(body.Current, body.New); err != nil {
			if err == auth.ErrBadCredentials {
				jsonErr(w, 403, "current password is wrong")
			} else {
				jsonErr(w, 400, err.Error())
			}
			return
		}
		setSession(w, r) // old sessions are now invalid; keep this one signed in
		writeJSON(w, 200, map[string]string{"status": "ok"})

	default:
		jsonErr(w, 404, "not found")
	}
}

// normCode makes setup codes forgiving to type: case, spaces and dashes are ignored.
func normCode(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}
