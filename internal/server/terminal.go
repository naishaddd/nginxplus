package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/naishaddd/nginxplus/internal/docker"
)

// Interactive web terminal: a WebSocket bridged to `docker exec /bin/sh` (TTY)
// in the target container via a hijacked Docker API connection.

var upgrader = websocket.Upgrader{
	CheckOrigin: checkSameOrigin,
}

func handleTerminal(w http.ResponseWriter, r *http.Request) {
	if !acquireTerminal() {
		http.Error(w, `{"error":"too many open terminal sessions"}`, http.StatusTooManyRequests)
		return
	}
	defer releaseTerminal()
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer ws.Close()

	client := docker.HTTPClient()
	createBody, _ := json.Marshal(map[string]interface{}{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          []string{"/bin/sh"},
	})
	createURL := fmt.Sprintf("http://localhost/v1.45/containers/%s/exec", docker.Target)
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := client.Do(createReq)
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mDocker error: %v\x1b[0m\r\n", err)))
		return
	}
	defer createResp.Body.Close()

	var execRes struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&execRes); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mFailed to decode exec id\x1b[0m\r\n"))
		return
	}
	execID := execRes.ID

	rawConn, err := docker.DialRaw()
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mDocker socket dial error: %v\x1b[0m\r\n", err)))
		return
	}
	defer rawConn.Close()

	hijackReq := fmt.Sprintf("POST /v1.45/exec/%s/start HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nUpgrade: tcp\r\nConnection: Upgrade\r\nContent-Length: 29\r\n\r\n{\"Detach\":false,\"Tty\":true}\r\n", execID)
	if _, err := rawConn.Write([]byte(hijackReq)); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mStart exec error: %v\x1b[0m\r\n", err)))
		return
	}

	rBuf := bufio.NewReader(rawConn)
	for {
		line, err := rBuf.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
	}

	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			rawConn.Close()
			ws.Close()
		})
	}

	go func() {
		defer closeBoth()
		buf := make([]byte, 4096)
		for {
			n, err := rBuf.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				if err := ws.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}

		if strings.HasPrefix(string(msg), `{"resize":`) {
			var resizeMsg struct {
				Resize struct {
					Cols uint `json:"cols"`
					Rows uint `json:"rows"`
				} `json:"resize"`
			}
			if err := json.Unmarshal(msg, &resizeMsg); err == nil {
				resizeURL := fmt.Sprintf("http://localhost/v1.45/exec/%s/resize?h=%d&w=%d", execID, resizeMsg.Resize.Rows, resizeMsg.Resize.Cols)
				resizeReq, _ := http.NewRequest("POST", resizeURL, nil)
				_, _ = client.Do(resizeReq)
				continue
			}
		}

		if _, err := rawConn.Write(msg); err != nil {
			break
		}
	}
	closeBoth()
}
