package telemetry

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/naishaddd/nginxplus/internal/docker"
)

func StubStatus() StubMetrics {
	var m StubMetrics
	// Fetch via curl to internal stub_status endpoint
	code, out, err := docker.Exec([]string{"curl", "-k", "-s", "https://127.0.0.1:8443/metrics"})
	if err != nil || code != 0 {
		return m
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Active connections:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				m.ActiveConnections, _ = strconv.Atoi(parts[2])
			}
		} else if strings.HasPrefix(line, "Reading:") {
			parts := strings.Fields(line)
			// Reading: 0 Writing: 1 Waiting: 0
			for i := 0; i < len(parts)-1; i++ {
				if parts[i] == "Reading:" {
					m.Reading, _ = strconv.Atoi(parts[i+1])
				} else if parts[i] == "Writing:" {
					m.Writing, _ = strconv.Atoi(parts[i+1])
				} else if parts[i] == "Waiting:" {
					m.Waiting, _ = strconv.Atoi(parts[i+1])
				}
			}
		} else if !strings.HasPrefix(line, "server") && len(strings.Fields(line)) == 3 {
			parts := strings.Fields(line)
			m.Accepts, _ = strconv.ParseInt(parts[0], 10, 64)
			m.Handled, _ = strconv.ParseInt(parts[1], 10, 64)
			m.Requests, _ = strconv.ParseInt(parts[2], 10, 64)
		}
	}
	return m
}
