package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/naishaddd/nginxplus/internal/docker"
	"github.com/naishaddd/nginxplus/internal/nginx"
	"github.com/naishaddd/nginxplus/internal/telemetry"
)

// ---------------------------------------------------------------------------
// Cached `nginx -T` analysis (invalidated after save / reload / rollback)
// ---------------------------------------------------------------------------

var analysisCache struct {
	sync.Mutex
	at       time.Time
	analysis nginx.Analysis
	raw      string
	err      error
}

const analysisTTL = 15 * time.Second

func cachedAnalysis() (nginx.Analysis, string, error) {
	analysisCache.Lock()
	defer analysisCache.Unlock()
	if !analysisCache.at.IsZero() && time.Since(analysisCache.at) < analysisTTL {
		return analysisCache.analysis, analysisCache.raw, analysisCache.err
	}
	_, out, err := docker.Exec([]string{"nginx", "-T"})
	analysisCache.at, analysisCache.raw, analysisCache.err = time.Now(), out, err
	if err == nil {
		analysisCache.analysis = nginx.ParseDump(out)
	}
	return analysisCache.analysis, out, err
}

func invalidateAnalysis() {
	analysisCache.Lock()
	analysisCache.at = time.Time{}
	analysisCache.Unlock()
}

// ---------------------------------------------------------------------------
// Server-Sent Events: one push channel for live dashboard metrics
// ---------------------------------------------------------------------------

func handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	window := 60
	switch r.URL.Query().Get("window") {
	case "5m":
		window = 300
	case "15m":
		window = 900
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // don't let a fronting nginx buffer SSE

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	stubEvery := 0
	var stub telemetry.StubMetrics
	for {
		if stubEvery%5 == 0 { // stub_status every 10s: it costs a docker exec
			stub = telemetry.StubStatus()
		}
		stubEvery++
		payload, _ := json.Marshal(map[string]interface{}{
			"traffic": telemetry.Traffic.Snapshot(window),
			"series":  telemetry.Traffic.Series(60),
			"stub":    stub,
			"window":  window,
		})
		if _, err := fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", payload); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}
