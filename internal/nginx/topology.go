package nginx

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// BuildTopology turns a parsed config into a directed graph:
//
//	client → listen port → server (vhost) → location → upstream → backend
//	                                                 ↘ static root / redirect / direct URL
//
// Every config-backed node carries file/line in Metadata so the UI can jump to source.
func BuildTopology(a Analysis) TopologyGraph {
	g := TopologyGraph{Nodes: []TopologyNode{}, Edges: []TopologyEdge{}}
	seen := map[string]bool{}
	node := func(id, label, typ string, meta map[string]interface{}) string {
		if !seen[id] {
			seen[id] = true
			g.Nodes = append(g.Nodes, TopologyNode{ID: id, Label: label, Type: typ, Status: "unknown", Metadata: meta})
		}
		return id
	}
	edgeSeen := map[string]bool{}
	edge := func(from, to, typ string) {
		if k := from + ">" + to; !edgeSeen[k] {
			edgeSeen[k] = true
			g.Edges = append(g.Edges, TopologyEdge{From: from, To: to, Type: typ})
		}
	}
	src := func(s Source, extra map[string]interface{}) map[string]interface{} {
		m := map[string]interface{}{"file": s.File, "line": s.Line}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	client := node("client", "Clients", "client", nil)

	upstreamIDs := map[string]string{}
	for _, up := range a.Upstreams {
		id := node("upstream:"+up.Name, up.Name, "upstream", src(up.Source, map[string]interface{}{"sub": fmt.Sprintf("%d server(s)", len(up.Servers))}))
		upstreamIDs[up.Name] = id
		for _, s := range up.Servers {
			addr := first(strings.Fields(s))
			b := node("backend:"+addr, addr, "backend", map[string]interface{}{"address": addr, "sub": strings.TrimSpace(strings.TrimPrefix(s, addr))})
			edge(id, b, "pool_member")
		}
	}

	for i, s := range a.Servers {
		names := strings.Join(s.ServerName, " ")
		if names == "" || names == `""` || names == "_" {
			names = "(catch-all)"
		}
		sub := strings.Join(s.Listen, " · ")
		if s.DefaultServer {
			sub += " · default"
		}
		srvID := node(fmt.Sprintf("server:%d", i), names, "server", src(s.Source, map[string]interface{}{"sub": sub, "ssl": s.SSL}))
		for _, l := range s.Listen {
			port := listenPort(l)
			edge(client, node("port:"+port, ":"+port, "port", map[string]interface{}{"sub": map[bool]string{true: "TLS", false: "HTTP"}[strings.Contains(l, "ssl") || port == "443"]}), "connects")
			edge("port:"+port, srvID, "routes_to")
		}
		if s.Return != "" {
			edge(srvID, node(fmt.Sprintf("return:%d", i), "return "+s.Return, "return", src(s.Source, nil)), "returns")
		}
		for j, l := range s.Locations {
			if l.Modifier == "@" {
				continue // named locations are only reached internally
			}
			locID := node(fmt.Sprintf("location:%d:%d", i, j), l.Path, "location", src(l.Source, map[string]interface{}{"match": l.Match, "modifier": l.Modifier}))
			edge(srvID, locID, "matches")
			for _, pp := range l.ProxyPass {
				if up, ok := upstreamIDs[proxyHost(pp)]; ok {
					edge(locID, up, "proxies")
				} else {
					edge(locID, node("proxy:"+pp, pp, "backend", map[string]interface{}{"sub": "direct proxy_pass"}), "proxies")
				}
			}
			switch {
			case l.Return != "":
				edge(locID, node(fmt.Sprintf("return:%d:%d", i, j), "return "+l.Return, "return", src(l.Source, nil)), "returns")
			case l.Alias != "":
				edge(locID, node("static:"+l.Alias, l.Alias, "static", map[string]interface{}{"sub": "alias"}), "serves")
			case len(l.ProxyPass) == 0:
				root := l.Root
				if root == "" {
					root = s.Root
				}
				if root == "" {
					root = "/usr/share/nginx/html"
				}
				edge(locID, node("static:"+root, root, "static", map[string]interface{}{"sub": "static files"}), "serves")
			}
		}
	}
	return g
}

func listenPort(l string) string {
	addr := first(strings.Fields(l))
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		addr = addr[i+1:]
	}
	if addr == "" {
		return "80"
	}
	return addr
}

// proxyHost returns the host part of a proxy_pass URL ("http://api_cluster/x" → "api_cluster").
func proxyHost(pp string) string {
	u, err := url.Parse(pp)
	if err != nil || u.Host == "" {
		return pp
	}
	return u.Hostname()
}

// ---------------------------------------------------------------------------
// Route tester: which server{} and location{} handle a request?
// Follows nginx's documented algorithm. Nested locations and `if` blocks are not evaluated.
// ---------------------------------------------------------------------------

type RouteMatch struct {
	Found         bool          `json:"found"`
	ServerIndex   int           `json:"server_index"`
	LocationIndex int           `json:"location_index"` // -1 when the server has no matching location
	Server        *ServerBlock  `json:"server,omitempty"`
	Location      *LocationInfo `json:"location,omitempty"`
	Steps         []string      `json:"steps"`
	Target        string        `json:"target"` // what finally handles it
}

func MatchRoute(a Analysis, host, port, path string) RouteMatch {
	rm := RouteMatch{ServerIndex: -1, LocationIndex: -1, Steps: []string{}}
	if port == "" {
		port = "80"
	}
	if path == "" {
		path = "/"
	}
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Split(host, ":")[0], "."))

	// 1. servers listening on the port
	var cands []int
	for i, s := range a.Servers {
		for _, l := range s.Listen {
			if listenPort(l) == port {
				cands = append(cands, i)
				break
			}
		}
	}
	if len(cands) == 0 {
		rm.Steps = append(rm.Steps, fmt.Sprintf("No server block listens on port %s.", port))
		return rm
	}
	rm.Steps = append(rm.Steps, fmt.Sprintf("%d server block(s) listen on port %s.", len(cands), port))

	// 2. server_name: exact → longest leading wildcard → longest trailing wildcard → first regex → default
	si, how := pickServer(a.Servers, cands, host)
	rm.ServerIndex = si
	s := a.Servers[si]
	rm.Server = &s
	rm.Found = true
	rm.Steps = append(rm.Steps, fmt.Sprintf("Host %q → server %q (%s, %s:%d).", host, strings.Join(s.ServerName, " "), how, s.File, s.Line))
	if s.Return != "" {
		rm.Target = "return " + s.Return
		rm.Steps = append(rm.Steps, "Server-level `return "+s.Return+"` runs before location matching.")
		return rm
	}

	// 3. location
	li, steps := pickLocation(s.Locations, path)
	rm.Steps = append(rm.Steps, steps...)
	if li < 0 {
		rm.Target = "server defaults (no location matched → 404 unless root serves it)"
		return rm
	}
	rm.LocationIndex = li
	l := s.Locations[li]
	rm.Location = &l
	switch {
	case len(l.ProxyPass) > 0:
		rm.Target = "proxy_pass " + l.ProxyPass[0]
	case l.Return != "":
		rm.Target = "return " + l.Return
	case l.Alias != "":
		rm.Target = "alias " + l.Alias
	default:
		root := l.Root
		if root == "" {
			root = s.Root
		}
		rm.Target = "static files from " + root
	}
	return rm
}

func pickServer(servers []ServerBlock, cands []int, host string) (int, string) {
	bestLead, bestLeadLen := -1, -1
	bestTrail, bestTrailLen := -1, -1
	for _, i := range cands {
		for _, n := range servers[i].ServerName {
			n = strings.ToLower(n)
			switch {
			case n == host:
				return i, "exact server_name match"
			case strings.HasPrefix(n, "*.") && strings.HasSuffix(host, n[1:]) && len(n) > bestLeadLen:
				bestLead, bestLeadLen = i, len(n)
			case strings.HasPrefix(n, ".") && (host == n[1:] || strings.HasSuffix(host, n)) && len(n) > bestLeadLen:
				bestLead, bestLeadLen = i, len(n)
			case strings.HasSuffix(n, ".*") && strings.HasPrefix(host, n[:len(n)-1]) && len(n) > bestTrailLen:
				bestTrail, bestTrailLen = i, len(n)
			}
		}
	}
	if bestLead >= 0 {
		return bestLead, "longest leading-wildcard server_name"
	}
	if bestTrail >= 0 {
		return bestTrail, "longest trailing-wildcard server_name"
	}
	for _, i := range cands {
		for _, n := range servers[i].ServerName {
			if strings.HasPrefix(n, "~") {
				if re, err := compileNginxRegex(strings.TrimPrefix(n, "~"), false); err == nil && re.MatchString(host) {
					return i, "first matching regex server_name"
				}
			}
		}
	}
	for _, i := range cands {
		if servers[i].DefaultServer {
			return i, "no name matched → default_server for this port"
		}
	}
	return cands[0], "no name matched → first server on this port (implicit default)"
}

func pickLocation(locs []LocationInfo, path string) (int, []string) {
	var steps []string
	best, bestLen := -1, -1
	for i, l := range locs {
		if l.Modifier == "=" && l.Match == path {
			return i, append(steps, fmt.Sprintf("Exact match `location = %s` wins immediately.", l.Match))
		}
	}
	for i, l := range locs {
		if (l.Modifier == "" || l.Modifier == "^~") && strings.HasPrefix(path, l.Match) && len(l.Match) > bestLen {
			best, bestLen = i, len(l.Match)
		}
	}
	if best >= 0 {
		steps = append(steps, fmt.Sprintf("Longest prefix match: `location %s`.", locs[best].Path))
		if locs[best].Modifier == "^~" {
			return best, append(steps, "It uses ^~, so regex locations are skipped.")
		}
	}
	// regexes in config order
	type rx struct{ i int }
	var order []rx
	for i, l := range locs {
		if l.Modifier == "~" || l.Modifier == "~*" {
			order = append(order, rx{i})
		}
	}
	sort.SliceStable(order, func(x, y int) bool { return order[x].i < order[y].i })
	for _, r := range order {
		l := locs[r.i]
		re, err := compileNginxRegex(l.Match, l.Modifier == "~*")
		if err != nil {
			steps = append(steps, fmt.Sprintf("Skipped regex `%s` (not RE2-compatible: %v).", l.Match, err))
			continue
		}
		if re.MatchString(path) {
			return r.i, append(steps, fmt.Sprintf("First matching regex `location %s` overrides the prefix match.", l.Path))
		}
	}
	if best >= 0 {
		return best, append(steps, "No regex matched, so the prefix match is used.")
	}
	return -1, append(steps, "No location matched.")
}
