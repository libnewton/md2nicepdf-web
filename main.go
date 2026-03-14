package main

import (
	"context"
	"log"
	"net/http"
	"os"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	authToken := os.Getenv("AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("AUTH_TOKEN environment variable is required")
	}

	dockerHost := env("DOCKER_HOST", "unix:///var/run/docker.sock")
	converterImage := env("CONVERTER_IMAGE", "custom/pdf")
	listenAddr := env("LISTEN_ADDR", ":5000")
	workDir := env("WORK_DIR", "/tmp/md2pdf")

	srv := NewServer(authToken, dockerHost, converterImage, workDir)

	pullCtx, pullCancel := context.WithTimeout(context.Background(), imagePullTimeout)
	defer pullCancel()
	if err := srv.ensureConverterImage(pullCtx); err != nil {
		log.Fatalf("Failed to ensure converter image: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /pdf", srv.HandlePDF)

	// Serve static files from web/ directory
	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/", fs)

	log.Printf("Starting md2pdf server on %s", listenAddr)
	log.Printf("Converter image: %s", converterImage)
	log.Printf("Docker host: %s", dockerHost)

	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
