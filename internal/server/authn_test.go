package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFirstRunSetupAndLogin(t *testing.T) {
	t.Setenv("NGINXPP_DATA_DIR", t.TempDir())
	t.Setenv("NGINXPP_SETUP_CODE", "ABCD-1234")
	InitAuth()
	h := Routes(fstest.MapFS{
		"index.html": {Data: []byte("app")},
		"setup.html": {Data: []byte("setup")},
		"login.html": {Data: []byte("login")},
	})
	do := func(method, path, body string, cookies []*http.Cookie, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, c := range cookies {
			req.AddCookie(c)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if r := do("GET", "/", "", nil, nil); r.Code != 302 || r.Header().Get("Location") != "/setup.html" {
		t.Fatalf("before setup: %d %s", r.Code, r.Header().Get("Location"))
	}
	if r := do("GET", "/api/nginx/files", "", nil, nil); r.Code != 503 {
		t.Fatalf("api before setup: %d", r.Code)
	}
	if r := do("POST", "/api/auth/setup", `{"setupCode":"nope","username":"admin","password":"long enough pw"}`, nil, nil); r.Code != 403 {
		t.Fatalf("wrong code: %d", r.Code)
	}
	if r := do("POST", "/api/auth/setup", `{"setupCode":"abcd 1234","username":"admin","password":"short"}`, nil, nil); r.Code != 400 {
		t.Fatalf("weak password: %d", r.Code)
	}
	r := do("POST", "/api/auth/setup", `{"setupCode":"abcd 1234","username":"admin","password":"long enough pw"}`, nil, nil)
	if r.Code != 200 || len(r.Result().Cookies()) == 0 {
		t.Fatalf("setup: %d %s", r.Code, r.Body)
	}
	session := r.Result().Cookies()
	if r := do("POST", "/api/auth/setup", `{"setupCode":"abcd 1234","username":"x","password":"another long pw"}`, nil, nil); r.Code != 409 {
		t.Fatalf("second setup: %d", r.Code)
	}

	if r := do("GET", "/", "", session, nil); r.Code != 200 {
		t.Fatalf("index with session: %d", r.Code)
	}
	if r := do("GET", "/", "", nil, nil); r.Code != 302 || !strings.HasPrefix(r.Header().Get("Location"), "/login.html") {
		t.Fatalf("index without session: %d %s", r.Code, r.Header().Get("Location"))
	}
	if r := do("POST", "/api/auth/login", `{"username":"admin","password":"wrong password"}`, nil, nil); r.Code != 401 {
		t.Fatalf("bad login: %d", r.Code)
	}
	r = do("POST", "/api/auth/login", `{"username":"admin","password":"long enough pw"}`, nil, nil)
	if r.Code != 200 {
		t.Fatalf("login: %d", r.Code)
	}
	c := r.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags: %+v", c)
	}
	if r := do("POST", "/api/nginx/reload", "", session, map[string]string{"Origin": "https://evil.example"}); r.Code != 403 {
		t.Fatalf("cross-origin write: %d", r.Code)
	}
	if r := do("POST", "/api/auth/password", `{"current":"long enough pw","new":"brand new password"}`, session, nil); r.Code != 200 {
		t.Fatalf("change password: %d %s", r.Code, r.Body)
	}
	if r := do("GET", "/", "", session, nil); r.Code != 302 {
		t.Fatalf("old session should be invalid after password change: %d", r.Code)
	}
}
