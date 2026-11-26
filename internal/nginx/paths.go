package nginx

import (
	"path"
	"strings"
)

// ---------------------------------------------------------------------------
// Path & ID validation
// ---------------------------------------------------------------------------

// safeNginxPath cleans p and ensures it stays inside /etc/nginx/.
func SafePath(p string) (string, bool) {
	if p == "" || strings.ContainsRune(p, 0) {
		return "", false
	}
	clean := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if !strings.HasPrefix(clean, "/etc/nginx/") || strings.HasPrefix(clean, "/etc/nginx/.snapshots") {
		return "", false
	}
	return clean, true
}
