package telemetry

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/naishaddd/nginxplus/internal/nginx"
)

// Probes upstream health via TCP dial. All nodes are probed concurrently
// (bounded to 32 in flight), each with a 1s timeout, so total latency is
// roughly one timeout instead of N of them.
func ProbeUpstreams(upstreams []nginx.UpstreamPool) []UpstreamGroupHealth {
	results := make([]UpstreamGroupHealth, len(upstreams))
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup

	for gi, up := range upstreams {
		grp := &results[gi]
		grp.Name = up.Name
		grp.Nodes = make([]UpstreamNodeHealth, 0, len(up.Servers))
		for _, rawServer := range up.Servers {
			parts := strings.Fields(rawServer)
			if len(parts) == 0 {
				continue
			}
			node := UpstreamNodeHealth{
				Address: parts[0],
				Backup:  strings.Contains(rawServer, "backup"),
				Weight:  1,
				Status:  "down",
				Latency: "unreachable",
			}
			if strings.Contains(rawServer, " down") {
				node.Status, node.Latency = "drained", "disabled"
			}
			grp.Nodes = append(grp.Nodes, node)
		}
		for ni := range grp.Nodes {
			n := &grp.Nodes[ni]
			if n.Status == "drained" {
				continue
			}
			grp.Total++
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				start := time.Now()
				conn, err := net.DialTimeout("tcp", n.Address, time.Second)
				if err == nil {
					conn.Close()
					n.Status = "healthy"
					n.Latency = fmt.Sprintf("%dms", time.Since(start).Milliseconds())
				}
			}()
		}
	}
	wg.Wait()
	for gi := range results {
		for _, n := range results[gi].Nodes {
			if n.Status == "healthy" {
				results[gi].Healthy++
			}
		}
	}
	return results
}
