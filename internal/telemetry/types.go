// Package telemetry ingests the access log as a live stream into per-second
// buckets (1m/5m/15m windows), reads stub_status, and probes upstream servers.
package telemetry

// --- Metrics & Traffic State (Step 2 & Step 5) ---

type StubMetrics struct {
	ActiveConnections int   `json:"active_connections"`
	Accepts           int64 `json:"accepts"`
	Handled           int64 `json:"handled"`
	Requests          int64 `json:"requests"`
	Reading           int   `json:"reading"`
	Writing           int   `json:"writing"`
	Waiting           int   `json:"waiting"`
}

type UpstreamNodeHealth struct {
	Address string `json:"address"`
	Status  string `json:"status"` // healthy, down, drained
	Latency string `json:"latency"`
	Backup  bool   `json:"backup"`
	Weight  int    `json:"weight"`
}

type UpstreamGroupHealth struct {
	Name    string               `json:"name"`
	Nodes   []UpstreamNodeHealth `json:"nodes"`
	Healthy int                  `json:"healthy_count"`
	Total   int                  `json:"total_count"`
}

type TrafficMetrics struct {
	TotalRequests int            `json:"total_requests"`
	Status2xx     int            `json:"status_2xx"`
	Status3xx     int            `json:"status_3xx"`
	Status4xx     int            `json:"status_4xx"`
	Status5xx     int            `json:"status_5xx"`
	QPS           float64        `json:"qps"`
	TopEndpoints  []EndpointStat `json:"top_endpoints"`
	TopIPs        []IPStat       `json:"top_ips"`
}

type EndpointStat struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type IPStat struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}
