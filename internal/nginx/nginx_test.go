package nginx

import (
	"strings"
	"testing"
)

func TestLineDiff(t *testing.T) {
	d := LineDiff("a\nb\nc\n", "a\nB\nc\nd\n")
	if d.Additions != 2 || d.Deletions != 1 {
		t.Fatalf("got +%d -%d: %+v", d.Additions, d.Deletions, d.Lines)
	}
	if d.Lines[0].Op != " " || d.Lines[0].Text != "a" {
		t.Fatalf("first line should be context: %+v", d.Lines[0])
	}
}

func TestLineDiffIdentical(t *testing.T) {
	if d := LineDiff("x\ny", "x\ny\n"); d.Additions+d.Deletions != 0 {
		t.Fatalf("expected no changes: %+v", d)
	}
}

func rules(f []LintFinding) string {
	var r []string
	for _, x := range f {
		r = append(r, x.Rule)
	}
	return strings.Join(r, ",")
}

func TestLintRules(t *testing.T) {
	cfg := `
server {
    listen 443 ssl;
    ssl_protocols TLSv1 TLSv1.2;
    server_tokens on;
    add_header X-Frame-Options DENY;

    location /api {
        proxy_pass http://backend/;
    }
    location /v2/ {
        proxy_pass http://backend/v2;
        proxy_set_header X-Real-IP $remote_addr;
        add_header X-Debug 1;
    }
    location /files/ {
        autoindex on;
        if ($arg_x) { proxy_pass http://other; }
    }
    location /ok/ { proxy_pass http://backend/ok/; proxy_set_header Host $host; }
}`
	got := rules(Lint(cfg))
	for _, want := range []string{"weak-tls", "server-tokens", "proxy-pass-slash", "proxy-host-header", "add-header-inheritance", "autoindex", "if-in-location"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing rule %s in %s", want, got)
		}
	}
	if n := strings.Count(got, "proxy-pass-slash"); n != 2 {
		t.Errorf("expected 2 proxy-pass-slash findings, got %d (%s)", n, got)
	}
}

func TestLintCleanConfig(t *testing.T) {
	cfg := `server { listen 80; server_tokens off; location / { proxy_pass http://app; proxy_set_header Host $host; } }`
	if f := Lint(cfg); len(f) != 0 {
		t.Fatalf("expected no findings, got %+v", f)
	}
}

func TestLintLineNumbers(t *testing.T) {
	f := Lint("http {\n\n  ssl_protocols SSLv3;\n}")
	if len(f) != 1 || f[0].Line != 3 {
		t.Fatalf("got %+v", f)
	}
}

func TestSplitSnippet(t *testing.T) {
	h, s := SplitSnippet("limit_req_zone $binary_remote_addr zone=z:10m rate=5r/s;\nlocation /login {\n    limit_req zone=z;\n}\n")
	if !strings.Contains(h, "limit_req_zone") || strings.Contains(h, "location") {
		t.Fatalf("http part wrong: %q", h)
	}
	if !strings.Contains(s, "location /login") || strings.Contains(s, "limit_req_zone") {
		t.Fatalf("server part wrong: %q", s)
	}
	h, s = SplitSnippet("upstream app {\n  server 10.0.0.1:80;\n}\nlocation / { proxy_pass http://app; }")
	if !strings.Contains(h, "server 10.0.0.1") || !strings.Contains(s, "proxy_pass") {
		t.Fatalf("upstream block split wrong: %q | %q", h, s)
	}
}

func TestSafeNginxPath(t *testing.T) {
	cases := map[string]bool{
		"/etc/nginx/nginx.conf":            true,
		"/etc/nginx/conf.d/a.conf":         true,
		"/etc/nginx/../passwd":             false,
		"/etc/nginx/.snapshots/x":          false,
		"/etc/nginxevil/a":                 false,
		"etc/nginx/conf.d/../../../shadow": false,
	}
	for p, want := range cases {
		if _, ok := SafePath(p); ok != want {
			t.Errorf("%s: got %v want %v", p, ok, want)
		}
	}
}

func TestParseAccessLine(t *testing.T) {
	ip, path, code, ok := ParseAccessLine(`10.1.2.3 - - [21/Nov/2026:10:00:00 +0000] "POST /login HTTP/1.1" 429 0 "-" "x"`)
	if !ok || ip != "10.1.2.3" || path != "/login" || code != 429 {
		t.Fatalf("got %q %q %d %v", ip, path, code, ok)
	}
	if _, _, _, ok := ParseAccessLine("not a log line"); ok {
		t.Fatal("expected no match")
	}
}
