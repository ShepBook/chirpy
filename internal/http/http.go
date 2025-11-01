package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// methodRestriction returns a handler that validates the request method
// and returns HTTP 405 with Allow header if the method doesn't match
func methodRestriction(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

type Server struct {
	httpSrv *http.Server
	mux     *http.ServeMux
}

// NewWithConfig creates a server with custom handler configuration
func NewWithConfig(appHandler http.Handler) *Server {
	const port = "8080"

	mux := http.NewServeMux()

	mux.HandleFunc("/", handleHome)
	mux.Handle("/app/", appHandler)
	mux.HandleFunc("/api/healthz", methodRestriction("GET", handleHealthz))
	mux.HandleFunc("/api/validate_chirp", methodRestriction("POST", HandleValidateChirp))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpSrv: srv,
		mux:     mux,
	}
}

func New() *Server {
	const filepathRoot = "."

	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	return NewWithConfig(fileServer)
}

func (server *Server) Mux() *http.ServeMux {
	return server.mux
}

func (server *Server) ListenAndServe() error {
	return server.httpSrv.ListenAndServe()
}

func (server *Server) Shutdown(ctx context.Context) error {
	return server.httpSrv.Shutdown(ctx)
}

func handleHome(writer http.ResponseWriter, req *http.Request) {
	http.ServeFile(writer, req, "index.html")
}

func handleHealthz(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

// Request/Response structures for chirp validation

type validateChirpRequest struct {
	Body string `json:"body"`
}

type validateChirpResponse struct {
	Valid bool `json:"valid"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// HandleValidateChirp validates that a chirp is within the allowed character limit
func HandleValidateChirp(w http.ResponseWriter, r *http.Request) {
	var req validateChirpRequest

	// Decode the JSON request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorResponse{Error: "Invalid JSON"})
		return
	}

	// Validate chirp length using runes for proper Unicode support
	if len([]rune(req.Body)) > 140 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorResponse{Error: "Chirp is too long"})
		return
	}

	// Valid chirp
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(validateChirpResponse{Valid: true})
}
