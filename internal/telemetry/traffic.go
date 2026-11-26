package telemetry

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/naishaddd/nginxplus/internal/config"
	"github.com/naishaddd/nginxplus/internal/docker"
	"github.com/naishaddd/nginxplus/internal/nginx"
)

// ---------------------------------------------------------------------------
// Streaming access-log ingestion
//
// Each log line is read exactly once from a live stream:
//   - if /var/log/nginx/access.log is a symlink to /dev/stdout (the official
//     nginx image), we follow the Docker logs API;
//   - otherwise we stream `tail -n0 -F <file>` through docker exec.
// Lines are counted into 1-second buckets in a 15-minute ring buffer, so the
// 1m / 5m / 15m windows are real and nothing is counted twice.
// ---------------------------------------------------------------------------

const (
	ringSeconds      = 15 * 60
	maxKeysPerBucket = 256 // bounds memory under path/IP floods
)

var accessLogPath = config.Env("NGINXPP_ACCESS_LOG", "/var/log/nginx/access.log")

type bucket struct {
	sec            int64
	total          int
	s2, s3, s4, s5 int
	paths, ips     map[string]int
}

type trafficRing struct {
	mu      sync.Mutex
	buckets [ringSeconds]bucket
}

var Traffic = &trafficRing{}

func (t *trafficRing) Record(now time.Time, ip, path string, code int) {
	sec := now.Unix()
	t.mu.Lock()
	defer t.mu.Unlock()
	b := &t.buckets[sec%ringSeconds]
	if b.sec != sec { // recycle a stale slot
		*b = bucket{sec: sec, paths: map[string]int{}, ips: map[string]int{}}
	}
	b.total++
	switch {
	case code >= 500:
		b.s5++
	case code >= 400:
		b.s4++
	case code >= 300:
		b.s3++
	case code >= 200:
		b.s2++
	}
	if _, ok := b.paths[path]; ok || len(b.paths) < maxKeysPerBucket {
		b.paths[path]++
	}
	if _, ok := b.ips[ip]; ok || len(b.ips) < maxKeysPerBucket {
		b.ips[ip]++
	}
}

// snapshot aggregates the last `window` seconds.
func (t *trafficRing) Snapshot(window int) TrafficMetrics {
	now := time.Now().Unix()
	paths, ips := map[string]int{}, map[string]int{}
	var m TrafficMetrics
	t.mu.Lock()
	for i := range t.buckets {
		b := &t.buckets[i]
		if b.sec == 0 || now-b.sec >= int64(window) || b.sec > now {
			continue
		}
		m.TotalRequests += b.total
		m.Status2xx += b.s2
		m.Status3xx += b.s3
		m.Status4xx += b.s4
		m.Status5xx += b.s5
		for k, v := range b.paths {
			paths[k] += v
		}
		for k, v := range b.ips {
			ips[k] += v
		}
	}
	t.mu.Unlock()

	m.QPS = float64(m.TotalRequests) / float64(window)
	m.TopEndpoints = topN(paths, 5, func(k string, v int) EndpointStat { return EndpointStat{Path: k, Count: v} })
	m.TopIPs = topN(ips, 5, func(k string, v int) IPStat { return IPStat{IP: k, Count: v} })
	return m
}

// series returns per-second request counts for the last n seconds (oldest first), for sparklines.
func (t *trafficRing) Series(n int) []int {
	now := time.Now().Unix()
	out := make([]int, n)
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := 0; i < n; i++ {
		sec := now - int64(n-1-i)
		if b := &t.buckets[sec%ringSeconds]; b.sec == sec {
			out[i] = b.total
		}
	}
	return out
}

func topN[T any](m map[string]int, n int, mk func(string, int) T) []T {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		out = append(out, mk(k, m[k]))
	}
	return out
}

func ingestLine(line string) {
	ip, path, code, ok := nginx.ParseAccessLine(line)
	if ok {
		Traffic.Record(time.Now(), ip, path, code)
	}
}

// startBackgroundTelemetry keeps one log stream open, reconnecting with backoff.
func Start() {
	go func() {
		backoff := time.Second
		for {
			started := time.Now()
			err := streamAccessLog()
			if time.Since(started) > 30*time.Second {
				backoff = time.Second
			}
			log.Printf("access log stream ended (%v); reconnecting in %s", err, backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
}

func streamAccessLog() error {
	_, target, _ := docker.Exec([]string{"readlink", accessLogPath})
	target = strings.TrimSpace(target)

	var body io.ReadCloser
	var err error
	if target == "/dev/stdout" {
		q := url.Values{"follow": {"1"}, "stdout": {"1"}, "stderr": {"0"}, "tail": {"0"}}
		u := fmt.Sprintf("http://localhost/v1.45/containers/%s/logs?%s", url.PathEscape(docker.Target), q.Encode())
		body, err = docker.Stream(http.MethodGet, u, nil)
	} else {
		body, err = docker.ExecStream([]string{"tail", "-n", "0", "-F", accessLogPath})
	}
	if err != nil {
		return err
	}
	defer body.Close()
	return docker.ScanStream(body, ingestLine)
}
