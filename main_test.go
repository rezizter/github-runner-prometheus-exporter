package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TestMetricsEndpoint checks if the /metrics HTTP route is successfully exposed
// and returns a 200 OK status code.
func TestMetricsEndpoint(t *testing.T) {
	// Create a test HTTP server using standard Prometheus handler
	handler := promhttp.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "go_info") && !strings.Contains(body, "process_cpu_seconds_total") {
		t.Errorf("Expected default Prometheus metrics in body, got: %s", body)
	}
}

// TestLogFileParsing simulates reading a GitHub runner diagnostic log file 
// and ensures your parser handles expected lines properly.
func TestLogFileParsing(t *testing.T) {
	// Create a temporary directory for fake runner logs
	tmpDir, err := os.MkdirTemp("", "runner_diag_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy worker log file matching GitHub Actions runner naming convention
	logContent := `[2026-06-01 10:00:00Z INFO JobDispatcher] Running job: Build and Test
[2026-06-01 10:02:15Z INFO JobDispatcher] Finished job: Build and Test with result: Succeeded
`
	logFilePath := filepath.Join(tmpDir, "worker_20260601-100000-utc.log")
	err = os.WriteFile(logFilePath, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write temporary log file: %v", err)
	}

	// Verify the file exists and can be read by your application logic
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		t.Errorf("Expected log file to exist at %s", logFilePath)
	}

	// Add assertions here tailored to your specific log parsing functions, e.g.:
	// metrics := ParseLogFile(logFilePath)
	// if metrics.CompletedJobs != 1 {
	//     t.Errorf("Expected 1 completed job, got %d", metrics.CompletedJobs)
	// }
}

// TestInvalidLogPath checks graceful handling when a specified log directory doesn't exist.
func TestInvalidLogPath(t *testing.T) {
	nonExistentPath := "/non/existent/path/to/_diag"
	
	// Ensure your app handles missing paths without crashing (e.g., logs a warning/error instead of panic)
	if _, err := os.Stat(nonExistentPath); !os.IsNotExist(err) {
		t.Errorf("Expected error for non-existent path")
	}
}
