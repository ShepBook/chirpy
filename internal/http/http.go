package http

import (
	"context"
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
