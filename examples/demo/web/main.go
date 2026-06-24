// A tiny demo frontend: serves the static files in ./public. devspanner runs it
// with this directory as the working dir, so the relative path resolves.
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "5173"
	}
	log.Printf("demo web listening on :%s", port)
	if err := http.ListenAndServe(":"+port, http.FileServer(http.Dir("public"))); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
