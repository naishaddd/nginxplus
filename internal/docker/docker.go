// Package docker is a minimal Docker Engine API client over the unix socket:
// exec (buffered and streaming), log streaming, and file upload via the archive API.
// All calls target the container named by TARGET_CONTAINER.
package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/naishaddd/nginxplus/internal/config"
)

var Target = config.Env("TARGET_CONTAINER", "my-nginx")

const Socket = "/var/run/docker.sock"

func HTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", Socket)
			},
		},
		Timeout: 30 * time.Second,
	}
}

func DialRaw() (net.Conn, error) {
	return net.Dial("unix", Socket)
}

func Exec(cmd []string) (int, string, error) {
	client := HTTPClient()

	createBody, err := json.Marshal(map[string]interface{}{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          cmd,
	})
	if err != nil {
		return -1, "", err
	}

	createURL := fmt.Sprintf("http://localhost/v1.45/containers/%s/exec", Target)
	req, err := http.NewRequest("POST", createURL, bytes.NewReader(createBody))
	if err != nil {
		return -1, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return -1, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return -1, "", fmt.Errorf("exec create failed (%d): %s", resp.StatusCode, string(b))
	}

	var createResp struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return -1, "", err
	}

	startBody, err := json.Marshal(map[string]interface{}{
		"Detach": false,
		"Tty":    false,
	})
	if err != nil {
		return -1, "", err
	}

	startURL := fmt.Sprintf("http://localhost/v1.45/exec/%s/start", createResp.ID)
	reqStart, err := http.NewRequest("POST", startURL, bytes.NewReader(startBody))
	if err != nil {
		return -1, "", err
	}
	reqStart.Header.Set("Content-Type", "application/json")

	startResp, err := client.Do(reqStart)
	if err != nil {
		return -1, "", err
	}
	defer startResp.Body.Close()

	var outputBuf bytes.Buffer
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(startResp.Body, header)
		if err != nil {
			break
		}
		frameSize := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
		frame := make([]byte, frameSize)
		_, err = io.ReadFull(startResp.Body, frame)
		if err != nil {
			outputBuf.Write(frame)
			break
		}
		outputBuf.Write(frame)
	}

	inspectURL := fmt.Sprintf("http://localhost/v1.45/exec/%s/json", createResp.ID)
	inspectResp, err := client.Get(inspectURL)
	if err == nil {
		defer inspectResp.Body.Close()
		var inspectData struct {
			ExitCode int `json:"ExitCode"`
		}
		if err := json.NewDecoder(inspectResp.Body).Decode(&inspectData); err == nil {
			return inspectData.ExitCode, outputBuf.String(), nil
		}
	}

	return 0, outputBuf.String(), nil
}

// ---------------------------------------------------------------------------
// Safe file writes via the Docker archive API (no shell involved)
// ---------------------------------------------------------------------------

func WriteFile(filePath, content string) error {
	dir, name := path.Split(filePath)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0644, Size: int64(len(content)), ModTime: time.Now(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}

	u := fmt.Sprintf("http://localhost/v1.45/containers/%s/archive?path=%s",
		url.PathEscape(Target), url.QueryEscape(dir))
	req, err := http.NewRequest(http.MethodPut, u, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("archive upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// dockerStream opens a long-lived request on the Docker socket (no client timeout).
func Stream(method, u string, payload []byte) (io.ReadCloser, error) {
	client := HTTPClient()
	client.Timeout = 0
	req, err := http.NewRequest(method, u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("docker %s %d: %s", u, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp.Body, nil
}

// execStream runs cmd in the target container and returns its live (multiplexed) output.
func ExecStream(cmd []string) (io.ReadCloser, error) {
	createBody, _ := json.Marshal(map[string]interface{}{"AttachStdout": true, "AttachStderr": false, "Cmd": cmd})
	resp, err := HTTPClient().Post(
		fmt.Sprintf("http://localhost/v1.45/containers/%s/exec", url.PathEscape(Target)),
		"application/json", bytes.NewReader(createBody))
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"Id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if err != nil || created.ID == "" {
		return nil, fmt.Errorf("exec create failed: %v", err)
	}
	return Stream(http.MethodPost, fmt.Sprintf("http://localhost/v1.45/exec/%s/start", created.ID), []byte(`{"Detach":false,"Tty":false}`))
}

// scanDockerStream demultiplexes Docker's 8-byte-header stream format and
// calls fn for each complete line. Falls back to raw lines for TTY containers.
func ScanStream(r io.Reader, fn func(string)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	peek, err := br.Peek(8)
	if err != nil {
		return err
	}
	multiplexed := (peek[0] == 1 || peek[0] == 2) && peek[1] == 0 && peek[2] == 0 && peek[3] == 0
	if !multiplexed {
		sc := bufio.NewScanner(br)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			fn(sc.Text())
		}
		return sc.Err()
	}

	var partial []byte
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(br, hdr); err != nil {
			return err
		}
		size := binary.BigEndian.Uint32(hdr[4:])
		frame := make([]byte, size)
		if _, err := io.ReadFull(br, frame); err != nil {
			return err
		}
		if hdr[0] != 1 { // only stdout carries access logs
			continue
		}
		partial = append(partial, frame...)
		for {
			i := bytes.IndexByte(partial, '\n')
			if i < 0 {
				break
			}
			fn(string(partial[:i]))
			partial = partial[i+1:]
		}
		if len(partial) > 1024*1024 { // runaway line guard
			partial = partial[:0]
		}
	}
}
