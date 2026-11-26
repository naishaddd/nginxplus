package nginx

import (
	"os"
	"strings"
	"testing"
)

func loadDump(t *testing.T) Analysis {
	t.Helper()
	b, err := os.ReadFile("testdata/dump.txt")
	if err != nil {
		t.Fatal(err)
	}
	return ParseDump(string(b))
}

func TestParseDump(t *testing.T) {
	a := loadDump(t)
	if len(a.Files) != 2 || a.ServerCount != 2 || a.UpstreamCount != 1 {
		t.Fatalf("files=%v servers=%d upstreams=%d", a.Files, a.ServerCount, a.UpstreamCount)
	}
	tls := a.Servers[1]
	if !tls.SSL || !tls.DefaultServer || tls.File != "/etc/nginx/conf.d/app.conf" || tls.Line != 7 {
		t.Fatalf("tls server wrong: %+v", tls.Source)
	}
	if len(tls.Locations) != 6 || tls.Locations[2].Modifier != "=" || tls.Locations[5].Modifier != "@" {
		t.Fatalf("locations wrong: %+v", tls.Locations)
	}
	if a.Upstreams[0].Name != "api_cluster" || len(a.Upstreams[0].Servers) != 2 || a.Upstreams[0].Line != 4 {
		t.Fatalf("upstream wrong: %+v", a.Upstreams[0])
	}
}

func TestBuildTopology(t *testing.T) {
	g := BuildTopology(loadDump(t))
	has := func(from, to string) bool {
		for _, e := range g.Edges {
			if e.From == from && strings.HasPrefix(e.To, to) {
				return true
			}
		}
		return false
	}
	for _, c := range [][2]string{{"client", "port:443"}, {"port:443", "server:1"}, {"location:1:1", "upstream:api_cluster"}, {"upstream:api_cluster", "backend:10.0.0.1:8080"}, {"server:0", "return:0"}, {"location:1:3", "static:/srv/static/"}} {
		if !has(c[0], c[1]) {
			t.Errorf("missing edge %s → %s", c[0], c[1])
		}
	}
	for _, n := range g.Nodes {
		if strings.Contains(n.Label, "@fallback") {
			t.Error("named location should not be a graph node")
		}
	}
}

func TestMatchRoute(t *testing.T) {
	a := loadDump(t)
	cases := []struct{ host, port, path, target string }{
		{"example.com", "80", "/x", "return 301"},
		{"example.com", "443", "/api/users", "proxy_pass http://api_cluster/"},
		{"shop.example.com", "443", "/health", "return 200 ok"},
		{"other.org", "443", "/static/logo.png", "alias /srv/static/"},     // ^~ beats the regex
		{"example.com", "443", "/img/a.PNG", "static files from /srv/www"}, // ~* regex wins
		{"example.com", "443", "/about", "static files from /srv/www"},
	}
	for _, c := range cases {
		m := MatchRoute(a, c.host, c.port, c.path)
		if !strings.HasPrefix(m.Target, c.target) {
			t.Errorf("%s:%s%s → %q, want %q\n%s", c.host, c.port, c.path, m.Target, c.target, strings.Join(m.Steps, "\n"))
		}
	}
	if m := MatchRoute(a, "example.com", "443", "/img/a.PNG"); m.Location == nil || m.Location.Modifier != "~*" {
		t.Errorf("expected regex location, got %+v", m.Location)
	}
	if m := MatchRoute(a, "x", "8080", "/"); m.Found {
		t.Error("nothing listens on 8080")
	}
}

// The demo stack (examples/demo) should produce a realistically large map.
// testdata/demo-dump.txt is real `nginx -T` output of examples/demo/conf.d.
func TestDemoDumpTopologySize(t *testing.T) {
	b, err := os.ReadFile("testdata/demo-dump.txt")
	if err != nil {
		t.Fatal(err)
	}
	a := ParseDump(string(b))
	g := BuildTopology(a)
	counts := map[string]int{}
	for _, n := range g.Nodes {
		counts[n.Type]++
	}
	t.Logf("nodes=%d edges=%d by type=%v", len(g.Nodes), len(g.Edges), counts)
	if len(g.Nodes) < 50 || counts["server"] < 8 || counts["upstream"] != 8 {
		t.Fatalf("demo map too small: %d nodes %v", len(g.Nodes), counts)
	}
	if m := MatchRoute(a, "shop.local", "80", "/api/checkout/pay"); m.Target != "proxy_pass http://payments/" {
		t.Fatalf("route: %+v", m)
	}
}
