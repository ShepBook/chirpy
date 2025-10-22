package http

import (
	"context"
	"net/http"
	"time"
)

type Server struct {
	httpSrv *http.Server
}

func New() *Server {
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{httpSrv: srv}
}

func (server *Server) ListenAndServe() error {
	return server.httpSrv.ListenAndServe()
}

func (server *Server) Shutdown(ctx context.Context) error {
	return server.httpSrv.Shutdown(ctx)
}
