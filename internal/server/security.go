package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/naishaddd/nginxplus/internal/config"
	"github.com/naishaddd/nginxplus/internal/docker"
)

// checkSameOrigin rejects WebSocket upgrades from foreign origins (cross-site WebSocket hijacking).
func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// saveConfigValidated writes content, runs `nginx -t`, and restores the previous
// content if validation fails. Returns the nginx -t output.
func saveConfigValidated(filePath, content string) (bool, string, error) {
	prevCode, prev, _ := docker.Exec([]string{"cat", filePath})
	existed := prevCode == 0

	if err := docker.WriteFile(filePath, content); err != nil {
		return false, "", err
	}
	code, out, err := docker.Exec([]string{"nginx", "-t"})
	if err == nil && code == 0 {
		invalidateAnalysis()
		return true, out, nil
	}
	// Roll back the bad write.
	if existed {
		_ = docker.WriteFile(filePath, prev)
	} else {
		_, _, _ = docker.Exec([]string{"rm", "-f", filePath})
	}
	return false, out, err
}

// ---------------------------------------------------------------------------
// Read-only mode (NGINXPP_READONLY=true): observability only, no mutations.
// ---------------------------------------------------------------------------

var readOnly = strings.EqualFold(config.Env("NGINXPP_READONLY", "false"), "true")

func enforceReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutating := r.Method != http.MethodGet && r.URL.Path != "/api/nginx/lint"
		if readOnly && (mutating || strings.HasPrefix(r.URL.Path, "/ws/terminal")) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"NGINX++ is running in read-only mode"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Terminal session cap (NGINXPP_MAX_TERMINALS, default 5)
// ---------------------------------------------------------------------------

var terminalSlots = make(chan struct{}, func() int {
	n, err := strconv.Atoi(config.Env("NGINXPP_MAX_TERMINALS", "5"))
	if err != nil || n < 1 {
		return 5
	}
	return n
}())

func acquireTerminal() bool {
	select {
	case terminalSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseTerminal() { <-terminalSlots }

// ReadOnly reports whether NGINXPP_READONLY is enabled.
func ReadOnly() bool { return readOnly }
