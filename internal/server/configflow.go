package server

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/naishaddd/nginxplus/internal/docker"
	"github.com/naishaddd/nginxplus/internal/nginx"
	"github.com/naishaddd/nginxplus/internal/snapshots"
)

// ---------------------------------------------------------------------------
// Dry-run & sandbox validation
// ---------------------------------------------------------------------------

// dryRunConfig writes content, runs `nginx -t`, and ALWAYS restores the
// original. nginx only reads config on reload, so live traffic is unaffected.
func dryRunConfig(filePath, content string) (bool, string, error) {
	prevCode, prev, _ := docker.Exec([]string{"cat", filePath})
	if err := docker.WriteFile(filePath, content); err != nil {
		return false, "", err
	}
	code, out, err := docker.Exec([]string{"nginx", "-t"})
	if prevCode == 0 {
		_ = docker.WriteFile(filePath, prev)
	} else {
		_, _, _ = docker.Exec([]string{"rm", "-f", filePath})
	}
	return err == nil && code == 0, out, err
}

const sandboxFile = "/etc/nginx/conf.d/zz-nginxpp-sandbox.conf"

// sandboxValidate tests a snippet inside a throwaway server block that listens
// on loopback only, then removes it. Never reloads nginx.
func sandboxValidate(httpPart, serverPart string) (bool, string) {
	content := httpPart + "\n"
	if strings.TrimSpace(serverPart) != "" {
		content += "server {\n    listen 127.0.0.1:65080;\n    server_name nginxpp-sandbox.invalid;\n" + serverPart + "\n}\n"
	}
	if err := docker.WriteFile(sandboxFile, content); err != nil {
		return false, err.Error()
	}
	code, out, _ := docker.Exec([]string{"nginx", "-t"})
	_, _, _ = docker.Exec([]string{"rm", "-f", sandboxFile})
	return code == 0, out
}

// ---------------------------------------------------------------------------
// HTTP routes
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readFileFromContainer(p string) (string, bool) {
	code, out, err := docker.Exec([]string{"cat", p})
	return out, err == nil && code == 0
}

func registerConfigRoutes(mux *http.ServeMux) {
	// List snapshots (GET) / create a snapshot of a file's current content (POST)
	mux.HandleFunc("/api/nginx/snapshots", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct{ Path, Message string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonErr(w, 400, err.Error())
				return
			}
			p, ok := nginx.SafePath(body.Path)
			if !ok {
				jsonErr(w, 400, "Only /etc/nginx/ paths allowed")
				return
			}
			content, ok := readFileFromContainer(p)
			if !ok {
				jsonErr(w, 404, "File not found")
				return
			}
			id, err := snapshots.Create(p, content, "admin", body.Message)
			if err != nil {
				jsonErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]string{"status": "ok", "snapshot": id})
			return
		}
		writeJSON(w, 200, map[string]interface{}{"status": "ok", "snapshots": snapshots.List()})
	})

	// Diff: snapshot (old) vs the file's current content (new)
	mux.HandleFunc("/api/nginx/snapshots/diff", func(w http.ResponseWriter, r *http.Request) {
		meta, content, err := snapshots.Read(r.URL.Query().Get("id"))
		if err != nil {
			jsonErr(w, 404, err.Error())
			return
		}
		current, _ := readFileFromContainer(meta.FilePath)
		writeJSON(w, 200, map[string]interface{}{"status": "ok", "snapshot": meta, "diff": nginx.LineDiff(content, current)})
	})

	// Rollback: validate, restore, reload
	mux.HandleFunc("/api/nginx/snapshots/rollback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonErr(w, 405, "POST required")
			return
		}
		meta, content, err := snapshots.Read(r.URL.Query().Get("id"))
		if err != nil {
			jsonErr(w, 404, err.Error())
			return
		}
		target, ok := nginx.SafePath(meta.FilePath)
		if !ok {
			jsonErr(w, 422, "Snapshot has no valid target file")
			return
		}
		if cur, ok := readFileFromContainer(target); ok {
			_, _ = snapshots.Create(target, cur, "admin", "Before rollback to "+meta.ID)
		}
		applyAndReload(w, target, content)
	})

	// Export all snapshots as .tar.gz
	mux.HandleFunc("/api/nginx/snapshots/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nginxpp-snapshots-%s.tar.gz"`, time.Now().Format("20060102-150405")))
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		_ = filepath.Walk(snapshots.Dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(snapshots.Dir, p)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			_ = tw.WriteHeader(&tar.Header{Name: "snapshots/" + filepath.ToSlash(rel), Mode: 0o640, Size: int64(len(b)), ModTime: info.ModTime()})
			_, _ = tw.Write(b)
			return nil
		})
		_ = tw.Close()
		_ = gz.Close()
	})

	// Preview a change: diff vs current, lint findings, dry-run nginx -t
	mux.HandleFunc("/api/nginx/file/preview", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Path, Content string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		p, ok := nginx.SafePath(body.Path)
		if !ok {
			jsonErr(w, 400, "Only /etc/nginx/ paths allowed")
			return
		}
		current, _ := readFileFromContainer(p)
		valid, out, err := dryRunConfig(p, body.Content)
		if err != nil && out == "" {
			out = err.Error()
		}
		writeJSON(w, 200, map[string]interface{}{
			"status": "ok", "path": p,
			"diff":  nginx.LineDiff(current, body.Content),
			"lint":  nginx.Lint(body.Content),
			"valid": valid, "output": out,
		})
	})

	// Read / save a config file. Save = snapshot → validated write → optional reload (auto-restore on failure)
	mux.HandleFunc("/api/nginx/file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			p, ok := nginx.SafePath(r.URL.Query().Get("path"))
			if !ok {
				jsonErr(w, 400, "Only /etc/nginx/ paths allowed")
				return
			}
			content, ok := readFileFromContainer(p)
			if !ok {
				jsonErr(w, 404, "File not found")
				return
			}
			writeJSON(w, 200, map[string]string{"path": p, "content": content})
			return
		}
		if r.Method != http.MethodPost {
			jsonErr(w, 405, "GET or POST")
			return
		}
		var body struct {
			Path, Content, Message string
			Reload                 bool
			Snapshot               *bool // default true; false skips the undo snapshot
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		p, ok := nginx.SafePath(body.Path)
		if !ok {
			jsonErr(w, 400, "Only /etc/nginx/ paths allowed")
			return
		}
		snapID := ""
		if cur, ok := readFileFromContainer(p); ok && (body.Snapshot == nil || *body.Snapshot) {
			msg := body.Message
			if msg == "" {
				msg = "Before save"
			} else {
				msg = "Before: " + msg
			}
			snapID, _ = snapshots.Create(p, cur, "admin", msg)
		}
		if body.Reload {
			applyAndReload(w, p, body.Content, "snapshot", snapID)
			return
		}
		valid, out, err := saveConfigValidated(p, body.Content)
		switch {
		case err != nil && !valid && out == "":
			jsonErr(w, 500, "Save failed: "+err.Error())
		case !valid:
			writeJSON(w, 422, map[string]string{"status": "invalid", "message": "nginx -t failed; previous content restored", "output": out})
		default:
			writeJSON(w, 200, map[string]string{"status": "ok", "message": "Saved and validated (not reloaded)", "snapshot": snapID, "output": out})
		}
	})

	// Lint arbitrary config text
	mux.HandleFunc("/api/nginx/lint", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Content string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]interface{}{"status": "ok", "findings": nginx.Lint(body.Content)})
	})

	// Validate a recipe/snippet in the sandbox
	mux.HandleFunc("/api/nginx/snippets/validate", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ HTTP, Server, Snippet string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		h, s := body.HTTP, body.Server
		if body.Snippet != "" {
			h, s = nginx.SplitSnippet(body.Snippet)
		}
		ok, out := sandboxValidate(h, s)
		writeJSON(w, 200, map[string]interface{}{"status": "ok", "syntax_valid": ok, "syntax_test": out, "lint": nginx.Lint(h + "\n" + s)})
	})
}

// applyAndReload: validated write → reload → if reload fails, restore the
// previous content and reload again. extra is optional key/value pairs.
func applyAndReload(w http.ResponseWriter, p, content string, extra ...string) {
	prev, existed := readFileFromContainer(p)
	valid, testOut, err := saveConfigValidated(p, content)
	if err != nil && !valid && testOut == "" {
		jsonErr(w, 500, "Save failed: "+err.Error())
		return
	}
	resp := map[string]interface{}{"path": p, "output": testOut}
	for i := 0; i+1 < len(extra); i += 2 {
		resp[extra[i]] = extra[i+1]
	}
	if !valid {
		resp["status"], resp["message"] = "invalid", "nginx -t failed; nothing changed"
		writeJSON(w, 422, resp)
		return
	}
	code, reloadOut, rerr := docker.Exec([]string{"nginx", "-s", "reload"})
	invalidateAnalysis()
	resp["reload_output"] = reloadOut
	if rerr != nil || code != 0 {
		if existed {
			_ = docker.WriteFile(p, prev)
		} else {
			_, _, _ = docker.Exec([]string{"rm", "-f", p})
		}
		_, _, _ = docker.Exec([]string{"nginx", "-s", "reload"})
		resp["status"], resp["message"] = "rolled_back", "Reload failed; previous config restored and reloaded"
		writeJSON(w, 500, resp)
		return
	}
	resp["status"], resp["message"] = "ok", "Saved, validated and reloaded"
	writeJSON(w, 200, resp)
}
