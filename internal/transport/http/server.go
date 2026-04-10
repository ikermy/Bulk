package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	cfg "github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/logging"
)

// Server wraps http.Server and provides Start/Shutdown helpers
type Server struct {
	srv *http.Server
}

// NewServer creates configured http.Server with router from this package
func NewServer(c *cfg.Config, deps *di.Deps) *Server {
	router := NewRouter(c, deps)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port),
		Handler:      router,
		ReadTimeout:  c.Server.ReadTimeout,
		WriteTimeout: c.Server.WriteTimeout,
	}

	return &Server{srv: srv}
}

// Start runs the server in a goroutine and logs via provided logger
func (s *Server) Start(logger logging.Logger) {
	go func() {
		logger.Info("starting server", "addr", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server failed", "error", err)
		}
	}()
}

// Shutdown attempts graceful shutdown within provided timeout
func (s *Server) Shutdown(ctx context.Context, timeout time.Duration) error {
	ctxShut, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.srv.Shutdown(ctxShut)
}
