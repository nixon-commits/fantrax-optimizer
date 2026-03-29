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
	projectionSystem string // default projection system from CLI flag
	mux              *http.ServeMux

	mu    sync.Mutex
	cache map[string]*cacheEntry // keyed by "projSystem|date"
}

type cacheEntry struct {
	result *pipeline.Result
	at     time.Time
}

const cacheTTL = 5 * time.Minute

// New creates a new server on the given port.
func New(port int, projectionSystem string) *Server {
	s := &Server{
		port:             port,
		projectionSystem: projectionSystem,
		mux:              http.NewServeMux(),
		cache:            make(map[string]*cacheEntry),
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
	s.mux.HandleFunc("/api/compare", s.handleCompare)
}

// getResult returns a cached or fresh pipeline result.
// projSystem overrides the default if non-empty.
// Drops the lock during the network fetch so other requests aren't blocked.
func (s *Server) getResult(date, projSystem string) (*pipeline.Result, error) {
	if projSystem == "" {
		projSystem = s.projectionSystem
	}
	key := projSystem + "|" + date

	// Check cache under lock.
	s.mu.Lock()
	if entry, ok := s.cache[key]; ok && time.Since(entry.at) < cacheTTL {
		result := entry.result
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()

	// Build input outside the lock.
	input := pipeline.Input{
		ProjectionSystem: projSystem,
	}

	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", date, err)
		}
		input.Date = t
	}

	log.Printf("fetching pipeline data for %s (%s)...", date, projSystem)
	result, err := pipeline.Run(input)
	if err != nil {
		return nil, err
	}

	// Store result under lock.
	s.mu.Lock()
	s.cache[key] = &cacheEntry{result: result, at: time.Now()}
	s.mu.Unlock()

	log.Printf("pipeline data cached for %s (%s)", date, projSystem)
	return result, nil
}
