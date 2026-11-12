
import os
import time
import logging
import requests
import urllib3

from prometheus_client import start_http_server, Gauge

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

# Configuration
GITHUB_TOKEN = os.getenv("GITHUB_TOKEN")
GITHUB_ORG = os.getenv("GITHUB_ORG", "")
REPOS_FILE = os.getenv("REPOS_FILE", "repos.txt")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "60"))
PORT = int(os.getenv("EXPORTER_PORT", "9105"))

API_URL = "https://api.github.com"

SESSION = requests.Session()
SESSION.verify = False
SESSION.headers.update({
    "Authorization": f"Bearer {GITHUB_TOKEN}",
    "Accept": "application/vnd.github+json",
    "X-GitHub-Api-Version": "2022-11-28",
})

# Metrics
RUNNER_STATUS = Gauge(
    "github_actions_runner_status",
    "Runner status: 1=online, 0=offline",
    ["org", "repo", "runner", "runner_id", "os", "scope"]
)

RUNNER_BUSY = Gauge(
    "github_actions_runner_busy",
    "Runner busy: 1=busy, 0=idle",
    ["org", "repo", "runner", "runner_id", "scope"]
)

RUNNER_INFO = Gauge(
    "github_actions_runner_info",
    "Runner information",
    ["org", "repo", "runner", "runner_id",
     "os", "architecture", "version", "labels", "scope"]
)

RUNNERS_TOTAL = Gauge(
    "github_actions_runners_total",
    "Total self-hosted runners",
    ["org", "repo", "scope"]
)

RUNNERS_ONLINE = Gauge(
    "github_actions_runners_online",
    "Online self-hosted runners",
    ["org", "repo", "scope"]
)

RUNNERS_OFFLINE = Gauge(
    "github_actions_runners_offline",
    "Offline self-hosted runners",
    ["org", "repo", "scope"]
)

API_ERRORS = Gauge(
    "github_actions_api_errors",
    "GitHub API error status (1=error, 0=success)",
    ["org", "repo", "scope"]
)


def load_repositories():
    """Load repositories from configuration file."""
    try:
        with open(REPOS_FILE, "r", encoding="utf-8") as file:
            return [
                line.strip()
                for line in file
                if line.strip() and not line.strip().startswith("#")
            ]
    except FileNotFoundError:
        logging.warning("Repository file %s not found", REPOS_FILE)
        return []


def get_runners(endpoint):
    """Fetch all runners with pagination."""
    runners = []
    page = 1

    while True:
        response = SESSION.get(
            endpoint,
            params={"per_page": 100, "page": page},
            timeout=30,
        )
        response.raise_for_status()

        data = response.json()
        runners.extend(data.get("runners", []))

        if len(runners) >= data.get("total_count", 0):
            break

        page += 1

    return runners


def collect_scope(org, repo, scope, endpoint):
    """Collect runner metrics for organization or repository."""
    runners = get_runners(endpoint)

    online = 0
    offline = 0

    for runner in runners:
        name = runner["name"]
        runner_id = str(runner["id"])
        os_name = runner.get("os", "unknown")
        status = runner.get("status", "offline")
        busy = runner.get("busy", False)
        version = runner.get("version", "unknown")

        labels = ",".join(
            label["name"] for label in runner.get("labels", [])
        )

        is_online = 1 if status == "online" else 0

        online += is_online
        offline += 1 - is_online

        RUNNER_STATUS.labels(
            org, repo, name, runner_id, os_name, scope
        ).set(is_online)

        RUNNER_BUSY.labels(
            org, repo, name, runner_id, scope
        ).set(1 if busy else 0)

        RUNNER_INFO.labels(
            org, repo, name, runner_id,
            os_name,
            runner.get("architecture", "unknown"),
            version,
            labels,
            scope,
        ).set(1)

    RUNNERS_TOTAL.labels(org, repo, scope).set(len(runners))
    RUNNERS_ONLINE.labels(org, repo, scope).set(online)
    RUNNERS_OFFLINE.labels(org, repo, scope).set(offline)

    API_ERRORS.labels(org, repo, scope).set(0)

    logging.info(
        "%s %s: %d runners (%d online, %d offline)",
        scope, repo, len(runners), online, offline
    )


def collect_metrics():
    # Clear runner metrics to remove deleted runners/repos
    RUNNER_STATUS.clear()
    RUNNER_BUSY.clear()
    RUNNER_INFO.clear()

    repositories = load_repositories()

    # Organization runners
    if GITHUB_ORG:
        try:
            collect_scope(
                GITHUB_ORG,
                "",
                "org",
                f"{API_URL}/orgs/{GITHUB_ORG}/actions/runners",
            )
        except Exception as exc:
            logging.error("Organization runners failed: %s", exc)
            API_ERRORS.labels(GITHUB_ORG, "", "org").set(1)

    # Repository runners
    for full_repo in repositories:
        try:
            owner, repo = full_repo.split("/", 1)

            collect_scope(
                owner,
                repo,
                "repo",
                f"{API_URL}/repos/{owner}/{repo}/actions/runners",
            )

        except Exception as exc:
            logging.error("Repository %s failed: %s", full_repo, exc)

            try:
                owner, repo = full_repo.split("/", 1)
                API_ERRORS.labels(owner, repo, "repo").set(1)
            except ValueError:
                logging.error("Invalid repository format: %s", full_repo)


def main():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    if not GITHUB_TOKEN:
        raise ValueError("GITHUB_TOKEN must be set")

    start_http_server(PORT)

    logging.info("Exporter listening on port %d", PORT)

    while True:
        collect_metrics()
        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()

