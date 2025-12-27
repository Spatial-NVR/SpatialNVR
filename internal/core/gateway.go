package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// APIGateway routes requests to plugins
type APIGateway struct {
	loader   *PluginLoader
	eventBus *EventBus
	router   *chi.Mux
	logger   *slog.Logger
}

// NewAPIGateway creates a new API gateway
func NewAPIGateway(loader *PluginLoader, eventBus *EventBus, logger *slog.Logger) *APIGateway {
	gw := &APIGateway{
		loader:   loader,
		eventBus: eventBus,
		logger:   logger.With("component", "api-gateway"),
	}
	gw.setupRouter()
	return gw
}

// setupRouter configures the HTTP router
// Note: When mounted via main.go's r.Mount("/", gateway.Handler()) within /api/v1,
// the gateway routes should NOT include the /api/v1 prefix
func (gw *APIGateway) setupRouter() {
	r := chi.NewRouter()

	// No middleware here - let the main router handle middleware
	// This avoids double-middleware when mounted

	// Legacy routes - route to appropriate plugins
	// These are mounted at /api/v1 in main.go, so /cameras becomes /api/v1/cameras
	r.Route("/cameras", gw.routeToPlugin("nvr-core-api"))
	r.Route("/events", gw.routeToPlugin("nvr-core-events"))
	r.Route("/recordings", gw.routeToPlugin("nvr-recording"))
	r.Route("/timeline", gw.routeToPlugin("nvr-recording"))
	r.Route("/config", gw.routeToPlugin("nvr-core-config"))

	// Spatial tracking routes - integrated into main API
	// This means spatial API is at /api/v1/spatial/* instead of separate port
	r.Route("/spatial", gw.routeToPlugin("nvr-spatial-tracking"))

	gw.router = r
}

// Handler returns the HTTP handler
func (gw *APIGateway) Handler() http.Handler {
	return gw.router
}

// routeToPlugin returns a handler that routes to a specific plugin
func (gw *APIGateway) routeToPlugin(pluginID string) func(chi.Router) {
	return func(r chi.Router) {
		handler := func(w http.ResponseWriter, req *http.Request) {
			lp, ok := gw.loader.GetPlugin(pluginID)
			if !ok {
				gw.respondError(w, http.StatusServiceUnavailable, fmt.Sprintf("plugin not available: %s", pluginID))
				return
			}

			if lp.State != PluginStateRunning {
				gw.respondError(w, http.StatusServiceUnavailable, fmt.Sprintf("plugin not running: %s", pluginID))
				return
			}

			pluginHandler := lp.Plugin.Routes()
			if pluginHandler == nil {
				gw.respondError(w, http.StatusNotFound, fmt.Sprintf("plugin has no routes: %s", pluginID))
				return
			}

			pluginHandler.ServeHTTP(w, req)
		}
		// Handle all methods for both root path and subpaths
		r.HandleFunc("/", handler)
		r.HandleFunc("/*", handler)
	}
}

// respondJSON sends a JSON response
func (gw *APIGateway) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func (gw *APIGateway) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
