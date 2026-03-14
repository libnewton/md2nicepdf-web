package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

const maxBodySize = 30 * 1024 * 1024 // 30 MB

// Server holds configuration for the HTTP handlers.
type Server struct {
	authToken      string
	dockerHost     string
	converterImage string
	workDir        string
}

// NewServer creates a new Server instance.
func NewServer(authToken, dockerHost, converterImage, workDir string) *Server {
	return &Server{
		authToken:      authToken,
		dockerHost:     dockerHost,
		converterImage: converterImage,
		workDir:        workDir,
	}
}

type pdfRequest struct {
	MD            string `json:"md"`
	NoPageNumbers bool   `json:"no_page_numbers,omitempty"`
}

type errorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// HandlePDF processes a markdown-to-PDF conversion request.
func (s *Server) HandlePDF(w http.ResponseWriter, r *http.Request) {
	// Auth check
	token := r.Header.Get("internal-auth")
	if token != s.authToken {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}

	// Limit body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
			Error:  "payload_too_large",
			Detail: "Maximum payload size is 30MB",
		})
		return
	}

	var req pdfRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:  "invalid_json",
			Detail: err.Error(),
		})
		return
	}

	if req.MD == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing_md"})
		return
	}

	log.Printf("Processing PDF request (%d bytes markdown, no_page_numbers=%v)", len(req.MD), req.NoPageNumbers)

	pdfData, err := s.convertMarkdownToPDF(req.MD, req.NoPageNumbers)
	if err != nil {
		log.Printf("PDF conversion failed: %v", err)
		detail := "PDF conversion failed. Please try again later."
		var compileErr *CompilationError
		if errors.As(err, &compileErr) {
			detail = compileErr.Msg
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error:  "conversion_failed",
			Detail: detail,
		})
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="document.pdf"`)
	w.WriteHeader(http.StatusOK)
	w.Write(pdfData)
}
