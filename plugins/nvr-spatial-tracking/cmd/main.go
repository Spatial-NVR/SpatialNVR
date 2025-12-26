package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/mattn/go-sqlite3"
	spatial "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-spatial-tracking"
)

func main() {
	// Get data path from environment or use default
	dataPath := os.Getenv("SPATIAL_DATA_PATH")
	if dataPath == "" {
		dataPath = "/tmp/nvr-spatial-data"
	}

	// Get listen address from environment or use default
	listenAddr := os.Getenv("SPATIAL_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":5010"
	}

	log.Printf("Starting Spatial Tracking plugin on %s", listenAddr)
	log.Printf("Data path: %s", dataPath)

	// Create data directory
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Open database
	dbPath := dataPath + "/spatial.db"
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create store and run migrations
	store := spatial.NewStore(db)
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Printf("Database initialized at %s", dbPath)

	// Create track manager with slog logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	trackManager := spatial.NewTrackManager(store, logger)

	// Start track manager
	trackCtx, trackCancel := context.WithCancel(ctx)
	go trackManager.Run(trackCtx)

	// Create HTTP router
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(corsMiddleware)

	// Setup API routes
	setupRoutes(router, store, trackManager)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Spatial Tracking API listening on %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	trackCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

// CORS middleware for development
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Setup routes without plugin runtime dependency
func setupRoutes(r chi.Router, store *spatial.Store, trackManager *spatial.TrackManager) {
	r.Route("/api/v1", func(r chi.Router) {
		// Maps
		r.Route("/maps", func(r chi.Router) {
			r.Get("/", handleListMaps(store))
			r.Post("/", handleCreateMap(store))
			r.Route("/{mapId}", func(r chi.Router) {
				r.Get("/", handleGetMap(store))
				r.Put("/", handleUpdateMap(store))
				r.Delete("/", handleDeleteMap(store))
				r.Get("/cameras", handleListPlacements(store))
				r.Post("/cameras", handleCreatePlacement(store))
				r.Post("/auto-detect-transitions", handleAutoDetectTransitions(store))
				r.Get("/analytics", handleGetAnalytics(store))
				r.Route("/cameras/{placementId}", func(r chi.Router) {
					r.Put("/", handleUpdatePlacement(store))
					r.Delete("/", handleDeletePlacement(store))
				})
			})
		})

		// Transitions
		r.Route("/transitions", func(r chi.Router) {
			r.Get("/", handleListTransitions(store))
			r.Post("/", handleCreateTransition(store))
			r.Route("/{transitionId}", func(r chi.Router) {
				r.Delete("/", handleDeleteTransition(store))
			})
		})

		// Tracks
		r.Route("/tracks", func(r chi.Router) {
			r.Get("/", handleListTracks(trackManager))
		})
	})
}
