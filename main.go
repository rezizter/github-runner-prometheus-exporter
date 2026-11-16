package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Configuration variables
var (
	githubToken  string
	githubOrg    string
	reposFile    string
	pollInterval int
	port         int
	apiURL       string
)

func getEnv(key, fallback string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if val, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return fallback
}

func parseFlags() {
	flag.StringVar(&githubToken, "token", getEnv("GITHUB_TOKEN", ""), "GitHub personal access token (env: GITHUB_TOKEN)")
	flag.StringVar(&githubOrg, "org", getEnv("GITHUB_ORG", ""), "GitHub organization name to monitor (env: GITHUB_ORG)")
	flag.StringVar(&reposFile, "repos-file", getEnv("REPOS_FILE", "repos.txt"), "Path to file containing owner/repo list (env: REPOS_FILE)")
	flag.IntVar(&pollInterval, "poll-interval", getEnvAsInt("POLL_INTERVAL", 60), "Polling interval in seconds (env: POLL_INTERVAL)")
	flag.IntVar(&port, "port", getEnvAsInt("EXPORTER_PORT", 9877), "HTTP port for Prometheus metrics endpoint (env: EXPORTER_PORT)")
	flag.StringVar(&apiURL, "api-url", getEnv("GITHUB_API_URL", "https://api.github.com"), "GitHub API base URL (env: GITHUB_API_URL)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GitHub Actions Self-Hosted Runner Prometheus Exporter\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options (CLI flags override environment variables):\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n  %s --token=ghp_xxx --org=my-org --port=9877\n", os.Args[0])
	}

	flag.Parse()
}

// Prometheus Metrics
var (
	runnerStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runner_status",
			Help: "Runner status: 1=online, 0=offline",
		},
		[]string{"org", "repo", "runner", "runner_id", "os", "scope"},
	)

	runnerBusy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runner_busy",
			Help: "Runner busy: 1=busy, 0=idle",
		},
		[]string{"org", "repo", "runner", "runner_id", "scope"},
	)

	runnerInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runner_info",
			Help: "Runner information",
		},
		[]string{"org", "repo", "runner", "runner_id", "os", "architecture", "version", "labels", "scope"},
	)

	runnersTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runners_total",
			Help: "Total self-hosted runners",
		},
		[]string{"org", "repo", "scope"},
	)

	runnersOnline = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runners_online",
			Help: "Online self-hosted runners",
		},
		[]string{"org", "repo", "scope"},
	)

	runnersOffline = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_runners_offline",
			Help: "Offline self-hosted runners",
		},
		[]string{"org", "repo", "scope"},
	)

	apiErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_actions_api_errors",
			Help: "GitHub API error status (1=error, 0=success)",
		},
		[]string{"org", "repo", "scope"},
	)
)

func init() {
	prometheus.MustRegister(runnerStatus)
	prometheus.MustRegister(runnerBusy)
	prometheus.MustRegister(runnerInfo)
	prometheus.MustRegister(runnersTotal)
	prometheus.MustRegister(runnersOnline)
	prometheus.MustRegister(runnersOffline)
	prometheus.MustRegister(apiErrors)
}

// GitHub API JSON Response Structs
type Label struct {
	Name string `json:"name"`
}

type Runner struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	OS           string  `json:"os"`
	Status       string  `json:"status"`
	Busy         bool    `json:"busy"`
	Version      string  `json:"version"`
	Architecture string  `json:"architecture"`
	Labels       []Label `json:"labels"`
}

type RunnerResponse struct {
	TotalCount int      `json:"total_count"`
	Runners    []Runner `json:"runners"`
}

// Load repositories from configuration file
func loadRepositories() []string {
	data, err := os.ReadFile(reposFile)
	if err != nil {
		log.Printf("Warning: Repository file %s not found or unreadable: %v", reposFile, err)
		return []string{}
	}

	var repos []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			repos = append(repos, trimmed)
		}
	}
	return repos
}

// fetchRunnerPage executes a single paginated request and ensures resp.Body is closed immediately
func fetchRunnerPage(client *http.Client, endpoint string, perPage, page int) (*RunnerResponse, error) {
	url := fmt.Sprintf("%s?per_page=%d&page=%d", endpoint, perPage, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "github-actions-runner-exporter")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var result RunnerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Fetch all runner pages from GitHub API with pagination
func getRunners(client *http.Client, endpoint string) ([]Runner, error) {
	var allRunners []Runner
	page := 1
	perPage := 100
	var totalCount *int

	for {
		log.Printf("Fetching %s page %d", endpoint, page)

		result, err := fetchRunnerPage(client, endpoint, perPage, page)
		if err != nil {
			return nil, err
		}

		if totalCount == nil {
			tc := result.TotalCount
			totalCount = &tc
			log.Printf("GitHub reports %d runners", *totalCount)
		}

		allRunners = append(allRunners, result.Runners...)
		log.Printf("Page %d returned %d runners; collected %d/%d", page, len(result.Runners), len(allRunners), *totalCount)

		if len(allRunners) >= *totalCount {
			break
		}

		if len(result.Runners) == 0 {
			log.Printf("Empty page received; stopping pagination")
			break
		}

		page++
	}

	log.Printf("Total runners retrieved: %d", len(allRunners))
	return allRunners, nil
}

// Collect runner metrics for a specific scope (org or repo)
func collectScope(client *http.Client, org, repo, scope, endpoint string) {
	runners, err := getRunners(client, endpoint)
	if err != nil {
		log.Printf("Error fetching %s %s: %v", scope, repo, err)
		apiErrors.WithLabelValues(org, repo, scope).Set(1)
		return
	}

	online := 0
	offline := 0

	for _, runner := range runners {
		name := runner.Name
		runnerID := strconv.FormatInt(runner.ID, 10)
		osName := runner.OS
		if osName == "" {
			osName = "unknown"
		}
		status := runner.Status
		if status == "" {
			status = "offline"
		}
		busy := runner.Busy
		version := runner.Version
		if version == "" {
			version = "unknown"
		}

		var labelNames []string
		for _, lbl := range runner.Labels {
			labelNames = append(labelNames, lbl.Name)
		}
		labelsStr := strings.Join(labelNames, ",")

		isOnline := 0.0
		if status == "online" {
			isOnline = 1.0
			online++
		} else {
			offline++
		}

		runnerStatus.WithLabelValues(org, repo, name, runnerID, osName, scope).Set(isOnline)

		busyVal := 0.0
		if busy {
			busyVal = 1.0
		}
		runnerBusy.WithLabelValues(org, repo, name, runnerID, scope).Set(busyVal)

		// Only expose runner info for repositories in repos.txt
		if scope == "repo" && repo != "" {
			arch := runner.Architecture
			if arch == "" {
				arch = "unknown"
			}
			runnerInfo.WithLabelValues(org, repo, name, runnerID, osName, arch, version, labelsStr
