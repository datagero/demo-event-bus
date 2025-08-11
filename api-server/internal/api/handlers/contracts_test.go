package handlers

import (
	"bufio"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFrontendBackendRouteContracts validates that all routes called by the frontend
// are actually implemented in the backend
func TestFrontendBackendRouteContracts(t *testing.T) {
	// Find frontend routes
	frontendRoutes, err := extractFrontendRoutes()
	if err != nil {
		t.Skipf("Could not extract frontend routes: %v", err)
		return
	}

	// Find backend routes
	backendRoutes, err := extractBackendRoutes()
	if err != nil {
		t.Skipf("Could not extract backend routes: %v", err)
		return
	}

	t.Logf("Found %d frontend routes and %d backend routes", len(frontendRoutes), len(backendRoutes))

	// Check each frontend route exists in backend
	var missingRoutes []string
	for _, frontendRoute := range frontendRoutes {
		found := false
		for _, backendRoute := range backendRoutes {
			if routesMatch(frontendRoute, backendRoute) {
				found = true
				break
			}
		}
		if !found {
			missingRoutes = append(missingRoutes, frontendRoute)
		}
	}

	if len(missingRoutes) > 0 {
		t.Errorf("Frontend calls routes that don't exist in backend:\n%s", strings.Join(missingRoutes, "\n"))
	}
}

// TestCriticalFrontendRoutes ensures critical routes are always available
func TestCriticalFrontendRoutes(t *testing.T) {
	th := setupTestHandlers()

	// Routes that frontend depends on and often break
	criticalRoutes := []struct {
		method string
		path   string
		desc   string
	}{
		{"POST", "/api/player/start", "Individual player recruitment (compatibility route)"},
		{"POST", "/api/players/quickstart", "Quickstart recruitment"},
		{"POST", "/api/master/one", "Send One button"},
		{"POST", "/api/master/start", "Start Quest Wave button"},
		{"GET", "/api/rabbitmq/derived/metrics", "Frontend metrics panel"},
		{"GET", "/api/rabbitmq/derived/scoreboard", "Frontend scoreboard"},
		{"GET", "/health", "Health check"},
	}

	// Setup all routes like the real server
	setupAllRoutes(th)

	for _, route := range criticalRoutes {
		t.Run(fmt.Sprintf("%s_%s", route.method, strings.ReplaceAll(route.path, "/", "_")), func(t *testing.T) {
			var w *httptest.ResponseRecorder

			switch route.method {
			case "GET":
				w = th.makeRequest("GET", route.path, nil)
			case "POST":
				// Use minimal valid payload for POST routes
				payload := map[string]interface{}{"test": true}
				if strings.Contains(route.path, "player") {
					payload = map[string]interface{}{
						"player": "test", "skills": "gather", "fail_pct": 0.1, "speed_multiplier": 1.0, "workers": 1,
					}
				} else if strings.Contains(route.path, "quickstart") {
					payload = map[string]interface{}{"preset": "alice_bob"}
				} else if strings.Contains(route.path, "master") {
					payload = map[string]interface{}{"quest_type": "gather"}
				}
				w = th.makeRequest("POST", route.path, payload)
			}

			// Route should exist (not 404) and not be a server error due to route issues
			assert.NotEqual(t, 404, w.Code, "Route %s (%s) should exist", route.path, route.desc)

			// Success, client error, or expected server error (but not 404 or 500 due to missing route)
			assert.Contains(t, []int{200, 201, 400, 401, 403, 501}, w.Code,
				"Route %s (%s) should be properly implemented, got status %d", route.path, route.desc, w.Code)
		})
	}
}

// Helper to setup all routes like the main application
func setupAllRoutes(th *TestHandlers) {
	// Copy route setup from routes.go
	api := th.router.Group("/api")

	// Health
	th.router.GET("/health", th.handlers.Health)
	api.GET("/health", th.handlers.Health)

	// Players
	players := api.Group("/players")
	{
		players.POST("/quickstart", th.handlers.QuickstartPlayers)
		players.POST("/start", th.handlers.StartPlayer)
	}

	// Compatibility route
	api.POST("/player/start", th.handlers.StartPlayer)

	// Messages
	api.POST("/master/start", th.handlers.StartMaster)
	api.POST("/master/one", th.handlers.SendOne)

	// RabbitMQ
	rabbitmq := api.Group("/rabbitmq")
	{
		derived := rabbitmq.Group("/derived")
		{
			derived.GET("/metrics", th.handlers.GetRabbitMQMetrics)
			derived.GET("/scoreboard", th.handlers.GetRabbitMQScoreboard)
		}
	}
}

// Extract routes from frontend JavaScript
func extractFrontendRoutes() ([]string, error) {
	frontendPath := filepath.Join("..", "..", "..", "legacy", "python", "app", "web_static", "index.html")

	file, err := os.Open(frontendPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var routes []string
	scanner := bufio.NewScanner(file)

	// Regex to match fetch('/api/...') calls
	fetchRegex := regexp.MustCompile(`fetch\(['"]([^'"]*api[^'"]*)['"]`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := fetchRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				route := match[1]
				// Clean up the route
				if strings.HasPrefix(route, "/") {
					route = route[1:] // Remove leading slash for consistency
				}
				routes = append(routes, route)
			}
		}
	}

	// Remove duplicates
	uniqueRoutes := make([]string, 0)
	seen := make(map[string]bool)
	for _, route := range routes {
		if !seen[route] {
			seen[route] = true
			uniqueRoutes = append(uniqueRoutes, route)
		}
	}

	return uniqueRoutes, scanner.Err()
}

// Extract routes from backend Go code
func extractBackendRoutes() ([]string, error) {
	routesPath := filepath.Join("..", "..", "routes.go")

	file, err := os.Open(routesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var routes []string
	scanner := bufio.NewScanner(file)

	// Regex to match route definitions like router.POST("/api/...", handler)
	routeRegex := regexp.MustCompile(`\.(GET|POST|PUT|DELETE)\("([^"]*)"`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := routeRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 2 {
				route := match[2]
				// Clean up the route
				if strings.HasPrefix(route, "/") {
					route = route[1:] // Remove leading slash for consistency
				}
				routes = append(routes, route)
			}
		}
	}

	return routes, scanner.Err()
}

// Check if frontend route matches backend route (considering parameters)
func routesMatch(frontendRoute, backendRoute string) bool {
	// Direct match
	if frontendRoute == backendRoute {
		return true
	}

	// Handle parameter patterns like :id
	backendPattern := strings.ReplaceAll(backendRoute, ":queue", "[^/]+")
	backendPattern = strings.ReplaceAll(backendPattern, ":id", "[^/]+")

	matched, _ := regexp.MatchString("^"+backendPattern+"$", frontendRoute)
	return matched
}

// TestRoutePerformance benchmarks critical routes
func TestRoutePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	th := setupTestHandlers()
	setupAllRoutes(th)

	criticalRoutes := []struct {
		method  string
		path    string
		payload interface{}
	}{
		{"GET", "/health", nil},
		{"GET", "/api/rabbitmq/derived/metrics", nil},
		{"POST", "/api/player/start", map[string]interface{}{
			"player": "perf_test", "skills": "gather", "fail_pct": 0.1, "speed_multiplier": 1.0, "workers": 1,
		}},
	}

	for _, route := range criticalRoutes {
		t.Run(route.path, func(t *testing.T) {
			// Warm up
			th.makeRequest(route.method, route.path, route.payload)

			// Measure
			start := time.Now()
			for i := 0; i < 10; i++ {
				w := th.makeRequest(route.method, route.path, route.payload)
				assert.NotEqual(t, 404, w.Code, "Route should exist")
			}
			duration := time.Since(start)

			avgDuration := duration / 10
			t.Logf("Average response time for %s %s: %v", route.method, route.path, avgDuration)

			// Assert reasonable performance (adjust thresholds as needed)
			assert.Less(t, avgDuration.Milliseconds(), int64(100), "Route should respond within 100ms")
		})
	}
}
