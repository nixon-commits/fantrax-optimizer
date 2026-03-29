package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nixon-commits/rosterbot/internal/pipeline"
	"github.com/nixon-commits/rosterbot/web"
)

// Server serves the web GUI and API endpoints.
type Server struct {
	port             int
	projectionSystem string
	mux              *http.ServeMux

	mu     sync.Mutex
	cache  *pipeline.Result
	cached time.Time
}

const cacheTTL = 5 * time.Minute

// New creates a new server on the given port.
func New(port int, projectionSystem string) *Server {
	s := &Server{
		port:             port,
		projectionSystem: projectionSystem,
		mux:              http.NewServeMux(),
	}
	s.routes()
	return s
}

// Start begins serving HTTP requests. Blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("server listening on http://localhost:%d", s.port)
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) routes() {
	// Serve embedded static files.
	staticFS, err := fs.Sub(web.StaticFiles, "static")
	if err != nil {
		log.Fatalf("embedded static files: %v", err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API endpoints.
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/projections", s.handleProjections)
	s.mux.HandleFunc("/api/blend-curve", s.handleBlendCurve)
	s.mux.HandleFunc("/api/lineup-diff", s.handleLineupDiff)
}

// getResult returns a cached or fresh pipeline result.
// Drops the lock during the network fetch so other requests (health, static files) aren't blocked.
func (s *Server) getResult(date string) (*pipeline.Result, error) {
	// Check cache under lock.
	s.mu.Lock()
	if s.cache != nil && time.Since(s.cached) < cacheTTL {
		if date == "" || s.cache.Date.Format("2006-01-02") == date {
			result := s.cache
			s.mu.Unlock()
			return result, nil
		}
	}
	s.mu.Unlock()

	// Build input outside the lock.
	input := pipeline.Input{
		ProjectionSystem: s.projectionSystem,
	}

	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", date, err)
		}
		input.Date = t
	}

	log.Printf("fetching pipeline data for %s...", date)
	result, err := pipeline.Run(input)
	if err != nil {
		return nil, err
	}

	// Store result under lock.
	s.mu.Lock()
	s.cache = result
	s.cached = time.Now()
	s.mu.Unlock()

	log.Printf("pipeline data cached (expires %s)", s.cached.Add(cacheTTL).Format("15:04:05"))
	return result, nil
}
