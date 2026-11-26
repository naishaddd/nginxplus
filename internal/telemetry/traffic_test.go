package telemetry

import (
	"testing"
	"time"
)

func TestTrafficRingWindowsAndTopN(t *testing.T) {
	r := &trafficRing{}
	now := time.Now()
	r.Record(now, "1.1.1.1", "/a", 200)
	r.Record(now, "1.1.1.1", "/a", 502)
	r.Record(now, "2.2.2.2", "/b", 404)
	r.Record(now.Add(-2*time.Minute), "3.3.3.3", "/old", 200) // outside 1m, inside 5m

	m1 := r.Snapshot(60)
	if m1.TotalRequests != 3 || m1.Status2xx != 1 || m1.Status4xx != 1 || m1.Status5xx != 1 {
		t.Fatalf("1m window wrong: %+v", m1)
	}
	if m1.TopEndpoints[0].Path != "/a" || m1.TopEndpoints[0].Count != 2 {
		t.Fatalf("top endpoints not sorted: %+v", m1.TopEndpoints)
	}
	if m5 := r.Snapshot(300); m5.TotalRequests != 4 {
		t.Fatalf("5m window wrong: %d", m5.TotalRequests)
	}
}

func TestIngestLineCombinedFormat(t *testing.T) {
	Traffic = &trafficRing{}
	ingestLine(`172.18.0.1 - - [21/Nov/2026:10:00:00 +0000] "GET /api/v1/users?id=1 HTTP/1.1" 200 612 "-" "curl/8.0"`)
	ingestLine(`garbage line`)
	if m := Traffic.Snapshot(60); m.TotalRequests != 1 || m.TopIPs[0].IP != "172.18.0.1" {
		t.Fatalf("parse failed: %+v", m)
	}
}
