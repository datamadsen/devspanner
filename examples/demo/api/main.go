// A tiny demo backend: a health endpoint, a heartbeat that keeps the captured log
// alive, and graceful shutdown so Ctrl-C (or devspanner's stop) exits cleanly.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		fmt.Fprintln(w, "devspanner demo api — try /health")
	})

	// A heartbeat so the log view shows live activity even with no traffic.
	go func() {
		for range time.Tick(5 * time.Second) {
			log.Println("heartbeat: api alive")
		}
	}()

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		log.Printf("demo api listening on :%s (health: /health)", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
