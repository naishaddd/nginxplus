// Package server wires the HTTP API, SSE metrics stream, web terminal and
// embedded UI, with authentication and read-only enforcement.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/naishaddd/nginxplus/internal/docker"
	"github.com/naishaddd/nginxplus/internal/nginx"
	"github.com/naishaddd/nginxplus/internal/telemetry"
)

type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Success  bool   `json:"success"`
}

// Routes builds the full HTTP handler: API, SSE stream, web terminal and the
// embedded UI, wrapped in read-only enforcement and authentication.
func Routes(static fs.FS) http.Handler {
	mux := http.NewServeMux()

	// 1. Container status API
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		client := docker.HTTPClient()
		inspectURL := fmt.Sprintf("http://localhost/v1.45/containers/%s/json", docker.Target)
		resp, err := client.Get(inspectURL)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": fmt.Sprintf("Container '%s' not found (%d)", docker.Target, resp.StatusCode),
			})
			return
		}

		var inspect struct {
			ID    string `json:"Id"`
			Name  string `json:"Name"`
			State struct {
				Status string `json:"Status"`
			} `json:"State"`
			Config struct {
				Image string `json:"Image"`
			} `json:"Config"`
			NetworkSettings struct {
				Ports map[string]interface{} `json:"Ports"`
			} `json:"NetworkSettings"`
			Created string `json:"Created"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": err.Error()})
			return
		}

		id := inspect.ID
		if len(id) > 12 {
			id = id[:12]
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "connected",
			"container": map[string]interface{}{
				"id":      id,
				"name":    strings.TrimPrefix(inspect.Name, "/"),
				"status":  inspect.State.Status,
				"image":   inspect.Config.Image,
				"ports":   inspect.NetworkSettings.Ports,
				"created": inspect.Created,
			},
		})
	})

	// 2. Nginx -T Parse API
	mux.HandleFunc("/api/nginx/parse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		analysis, output, err := cachedAnalysis()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"status":"error","message":%q}`, err.Error()), 500)
			return
		}
		exitCode := 0
		topology := nginx.BuildTopology(analysis)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"exit_code": exitCode,
			"analysis":  analysis,
			"topology":  topology,
			"raw_dump":  output,
		})
	})

	// 2b. Standalone Topology API (Step 1)
	mux.HandleFunc("/api/nginx/topology", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		analysis, _, err := cachedAnalysis()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"status":"error","message":%q}`, err.Error()), 500)
			return
		}
		topology := nginx.BuildTopology(analysis)
		json.NewEncoder(w).Encode(topology)
	})

	// 2b'. Route tester: which server/location handles host:port + path?
	mux.HandleFunc("/api/nginx/route", func(w http.ResponseWriter, r *http.Request) {
		analysis, _, err := cachedAnalysis()
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		q := r.URL.Query()
		writeJSON(w, 200, nginx.MatchRoute(analysis, q.Get("host"), q.Get("port"), q.Get("path")))
	})

	// 2c. Step 2: Upstreams Health Probe API
	mux.HandleFunc("/api/nginx/upstreams/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		analysis, _, err := cachedAnalysis()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		health := telemetry.ProbeUpstreams(analysis.Upstreams)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"upstreams": health,
		})
	})

	// 2d. Step 2 & 5: Live Metrics & Stub Status API
	mux.HandleFunc("/api/nginx/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stub := telemetry.StubStatus()

		window := map[string]int{"1m": 60, "5m": 300, "15m": 900}[r.URL.Query().Get("window")]
		if window == 0 {
			window = 60
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"stub":    stub,
			"traffic": telemetry.Traffic.Snapshot(window),
			"series":  telemetry.Traffic.Series(60),
			"window":  window,
		})
	})

	// 2e. Step 3: SSL Expiry Radar & Security Linter
	mux.HandleFunc("/api/nginx/ssl/audit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		certs, audit := scanSSLCertificates()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "ok",
			"certificates": certs,
			"audit":        audit,
		})
	})

	// 2f. Step 4: Snapshot List & Diff & Rollback
	// 2g. Snippet templates + sandbox nginx -t validation
	mux.HandleFunc("/api/nginx/snippets/generate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		explanation, snippet := nginx.SnippetTemplate(body.Prompt)

		// Sandbox test: http-level directives (limit_req_zone, map, …) go outside the
		// throwaway server block, everything else inside it.
		httpPart, serverPart := nginx.SplitSnippet(snippet)
		syntaxOK, testOut := sandboxValidate(httpPart, serverPart)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "ok",
			"prompt":       body.Prompt,
			"explanation":  explanation,
			"snippet":      snippet,
			"syntax_valid": syntaxOK,
			"syntax_test":  testOut,
		})
	})

	// 3. Test Syntax (nginx -t)
	mux.HandleFunc("/api/nginx/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		code, output, _ := docker.Exec([]string{"nginx", "-t"})
		json.NewEncoder(w).Encode(ExecResult{
			ExitCode: code,
			Output:   output,
			Success:  code == 0,
		})
	})

	// 4. Reload Nginx (nginx -s reload)
	mux.HandleFunc("/api/nginx/reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, 405)
			return
		}
		code, output, _ := docker.Exec([]string{"nginx", "-s", "reload"})
		invalidateAnalysis()
		msg := output
		if msg == "" && code == 0 {
			msg = "Nginx configuration reloaded successfully."
		}
		json.NewEncoder(w).Encode(ExecResult{
			ExitCode: code,
			Output:   msg,
			Success:  code == 0,
		})
	})

	// 5. List Files
	mux.HandleFunc("/api/nginx/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, output, err := docker.Exec([]string{"find", "/etc/nginx", "-maxdepth", "3", "-type", "f"})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var files []string
		for _, l := range lines {
			if l != "" && !strings.Contains(l, ".snapshots") {
				files = append(files, strings.TrimSpace(l))
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"files": files})
	})

	// 6. Read / Write Single File (with automated snapshot creation in Step 4)
	mux.HandleFunc("/api/nginx/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "100"
		}
		logType := r.URL.Query().Get("type")
		if logType == "" {
			logType = "access"
		}

		stdoutParam := "true"
		stderrParam := "false"
		if logType == "error" {
			stdoutParam = "false"
			stderrParam = "true"
		}

		client := docker.HTTPClient()
		logsURL := fmt.Sprintf("http://localhost/v1.45/containers/%s/logs?stdout=%s&stderr=%s&tail=%s",
			docker.Target, stdoutParam, stderrParam, url.QueryEscape(lines))

		resp, err := client.Get(logsURL)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"lines": []string{err.Error()}})
			return
		}
		defer resp.Body.Close()

		var logLines []string
		header := make([]byte, 8)
		for {
			_, err := io.ReadFull(resp.Body, header)
			if err != nil {
				break
			}
			frameSize := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
			frame := make([]byte, frameSize)
			_, err = io.ReadFull(resp.Body, frame)
			if err != nil {
				break
			}
			txt := strings.TrimSpace(string(frame))
			if txt != "" {
				for _, l := range strings.Split(txt, "\n") {
					if strings.TrimSpace(l) != "" {
						logLines = append(logLines, strings.TrimSpace(l))
					}
				}
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"type": logType, "lines": logLines})
	})

	// 7a. Config workflow: snapshots, diff, preview, save+reload, lint, sandbox
	registerConfigRoutes(mux)

	// 7b. Live metrics push channel (Server-Sent Events)
	mux.HandleFunc("/api/stream", handleStream)

	// 8. WebSocket Web Terminal with Docker Engine Hijack
	mux.HandleFunc("/ws/terminal", handleTerminal)

	// 9. Static UI Server
	mux.Handle("/", http.FileServer(http.FS(static)))

	return requireAuth(enforceReadOnly(mux))
}
