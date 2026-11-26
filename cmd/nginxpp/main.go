// Command nginxpp runs the NGINX++ dashboard: it attaches to an nginx
// container over the Docker socket and serves the UI, API and web terminal.
package main

import (
	"log"
	"net/http"
	"time"

	nginxpp "github.com/naishaddd/nginxplus"
	"github.com/naishaddd/nginxplus/internal/config"
	"github.com/naishaddd/nginxplus/internal/docker"
	"github.com/naishaddd/nginxplus/internal/server"
	"github.com/naishaddd/nginxplus/internal/telemetry"
)

func main() {
	server.InitAuth()
	telemetry.Start()

	port := config.Env("PORT", "8000")
	log.Printf("NGINX++ running on port %s (attached to %s, read-only=%v)", port, docker.Target, server.ReadOnly())
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: server.Routes(nginxpp.StaticFS()),
		// No Read/WriteTimeout: the SSE stream and web terminal are long-lived.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
