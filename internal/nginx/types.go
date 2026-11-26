// Package nginx contains pure, Docker-free nginx logic: parsing `nginx -T` output,
// the topology graph, the linter, line diffs, snippet templates and path validation.
// Everything here is unit-testable without a running nginx.
package nginx

// --- Data Models for Nginx Analysis ---

// Source points at a line in a config file (as printed by `nginx -T`).
type Source struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type LocationInfo struct {
	Source
	Modifier  string   `json:"modifier,omitempty"` // "", "=", "~", "~*", "^~", or "@" (named)
	Match     string   `json:"match"`              // the URI / regex / name
	Path      string   `json:"path"`               // modifier + match, as written
	ProxyPass []string `json:"proxy_pass"`
	Root      string   `json:"root,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	Return    string   `json:"return,omitempty"`
	TryFiles  string   `json:"try_files,omitempty"`
}

type ServerBlock struct {
	Source
	Listen        []string       `json:"listen"`
	ServerName    []string       `json:"server_name"`
	DefaultServer bool           `json:"default_server"`
	Root          string         `json:"root,omitempty"`
	Index         string         `json:"index,omitempty"`
	Return        string         `json:"return,omitempty"`
	SSL           bool           `json:"ssl"`
	Locations     []LocationInfo `json:"locations"`
	ProxyPasses   []string       `json:"proxy_passes"`
}

type UpstreamPool struct {
	Source
	Name    string   `json:"name"`
	Servers []string `json:"servers"`
}

type Analysis struct {
	ServerCount   int            `json:"server_count"`
	UpstreamCount int            `json:"upstream_count"`
	Servers       []ServerBlock  `json:"servers"`
	Upstreams     []UpstreamPool `json:"upstreams"`
	Files         []string       `json:"files"`
}

// Topology Data Models
type TopologyNode struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	Type     string                 `json:"type"`
	Status   string                 `json:"status"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}
