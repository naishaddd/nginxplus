// Package snapshots stores config snapshots on NGINX++'s data volume
// ($NGINXPP_DATA_DIR/snapshots/<id>/{content.conf,meta.json}).
package snapshots

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/naishaddd/nginxplus/internal/config"
)

// --- Config Snapshot Version Control (Step 4) ---

type Snapshot struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	FilePath  string `json:"file_path"`
	Author    string `json:"author"`
	Note      string `json:"note"`
}

var snapIDRe = regexp.MustCompile(`^snap-\d{8}-\d{6}(-\d+)?$`)

func ValidID(id string) bool { return snapIDRe.MatchString(id) }

// Config editing workflow:
//   preview (diff + lint + dry-run `nginx -t`) → save (validated) → reload →
//   automatic restore if the reload fails.
// Snapshots are stored on NGINX++'s own data volume, so they survive nginx
// container re-creation, and carry a user-supplied message.

// ---------------------------------------------------------------------------
// Snapshot store (local filesystem: $NGINXPP_DATA_DIR/snapshots/<id>/)
// ---------------------------------------------------------------------------

var Dir = initSnapshotDir()

func initSnapshotDir() string {
	dir := filepath.Join(config.Env("NGINXPP_DATA_DIR", "/data"), "snapshots")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fallback := filepath.Join(os.TempDir(), "nginxpp-snapshots")
		log.Printf("WARNING: cannot use %s (%v); snapshots go to %s and will NOT survive restarts. Mount a volume at /data.", dir, err, fallback)
		_ = os.MkdirAll(fallback, 0o750)
		return fallback
	}
	return dir
}

func Create(filePath, content, author, message string) (string, error) {
	now := time.Now()
	id := fmt.Sprintf("snap-%s-%03d", now.Format("20060102-150405"), now.Nanosecond()/1e6)
	dir := filepath.Join(Dir, id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	if message == "" {
		message = "Pre-save automatic snapshot"
	}
	meta := Snapshot{ID: id, Timestamp: now.Format(time.RFC3339), FilePath: filePath, Author: author, Note: message}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "content.conf"), []byte(content), 0o600); err != nil {
		return "", err
	}
	return id, os.WriteFile(filepath.Join(dir, "meta.json"), mb, 0o600)
}

func List() []Snapshot {
	entries, _ := os.ReadDir(Dir)
	snaps := make([]Snapshot, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir, e.Name(), "meta.json"))
		var s Snapshot
		if err == nil && json.Unmarshal(b, &s) == nil {
			snaps = append(snaps, s)
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ID > snaps[j].ID })
	return snaps
}

func Read(id string) (Snapshot, string, error) {
	var meta Snapshot
	if !ValidID(id) {
		return meta, "", fmt.Errorf("invalid snapshot id")
	}
	dir := filepath.Join(Dir, id)
	mb, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return meta, "", fmt.Errorf("snapshot not found")
	}
	if err := json.Unmarshal(mb, &meta); err != nil {
		return meta, "", err
	}
	content, err := os.ReadFile(filepath.Join(dir, "content.conf"))
	return meta, string(content), err
}
