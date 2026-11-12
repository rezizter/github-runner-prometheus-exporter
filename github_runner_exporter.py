
import os
import time
import logging
import requests

from prometheus_client import start_http_server, Gauge

# Configuration
GITHUB_TOKEN = os.environ["GITHUB_TOKEN"]
GITHUB_ORG = os.getenv("GITHUB_ORG")
GITHUB_REPOS = os.getenv("GITHUB_REPOS", "")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "60"))
EXPORTER_PORT = int(os.getenv("EXPORTER_PORT", "9877"))

GITHUB_API = os.getenv(
    "GITHUB_API_URL",
    "https://api.github.com"
)

API_VERSION = "2022-11-28"

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s"
)

logger = logging.getLogger(__name__)

session = requests.Session()
session.headers.update({
    "Authorization": f"Bearer {GITHUB_TOKEN}",
    "Accept": "application/vnd.github+json",
    "X-GitHub-Api-Version": API_VERSION,
})


# Prometheus metrics

RUNNER_STATUS = Gauge(
    "github_runner_status",
    "Runner status (1=online, 0=offline)",
    ["scope", "repository", "runner", "os"]
)

RUNNER_BUSY = Gauge(
    "github_runner_busy",
    "Runner busy status (1=busy, 0=idle)",
    ["scope", "repository", "runner"]
)

RUNNER_INFO = Gauge(
    "github_runner_info",
    "GitHub runner information",
    ["scope", "repository", "runner", "version", "labels"]
)

RUNNER_TOTAL = Gauge(
    "github_runners_total",
    "Total number of GitHub Actions runners",
    ["scope", "repository"]
)

RUNNER_ONLINE = Gauge(
    "github_runners_online",
    "Number of online GitHub Actions runners",
    ["scope", "repository"]
)

RUNNER_OFFLINE = Gauge(
    "github_runners_offline",
    "Number of offline GitHub Actions runners",
    ["scope", "repository"]
)

RUNNER_BUSY_COUNT = Gauge(
    "github_runners_busy",
    "Number of busy GitHub Actions runners",
    ["scope", "repository"]
)

RUNNER_IDLE = Gauge(
    "github_runners_idle",
    "Number of idle GitHub Actions runners",
    ["scope", "repository"]
)

EXPORTER_UP = Gauge(
    "github_exporter_up",
    "Whether the last GitHub API collection succeeded"
)

LAST_COLLECTION = Gauge(
    "github_exporter_last_collection_timestamp",
    "Unix timestamp of last successful collection"
)


def get_runners(endpoint):
    """Fetch all runners, handling API pagination."""

    runners = []
    page = 1

    while True:
        response = session.get(
            endpoint,
            params={
                "per_page": 100,
                "page": page
            },
            timeout=15
        )

        response.raise_for_status()

        data = response.json()
        batch = data.get("runners", [])

        runners.extend(batch)

        if len(batch) < 100:
            break

        page += 1

    return runners


def collect_runners(endpoint, scope, repository):
    """Collect runner metrics from GitHub."""

    runners = get_runners(endpoint)

    total = len(runners)
    online = 0
    offline = 0
    busy = 0
    idle = 0

    for runner in runners:

        name = runner.get("name", "unknown")
        status = runner.get("status", "offline")
        is_busy = runner.get("busy", False)
        os_name = runner.get("os", "unknown")
        version = runner.get("version", "unknown")

        labels = ",".join(
            sorted(
                label.get("name", "")
                for label in runner.get("labels", [])
            )
        )

        is_online = status == "online"

        if is_online:
            online += 1
        else:
            offline += 1

        if is_busy:
            busy += 1
        else:
            idle += 1

        RUNNER_STATUS.labels(
            scope, repository, name, os_name
        ).set(1 if is_online else 0)

        RUNNER_BUSY.labels(
            scope, repository, name
        ).set(1 if is_busy else 0)

        RUNNER_INFO.labels(
            scope, repository, name, version, labels
        ).set(1)

    # Aggregate metrics

    RUNNER_TOTAL.labels(scope, repository).set(total)
    RUNNER_ONLINE.labels(scope, repository).set(online)
    RUNNER_OFFLINE.labels(scope, repository).set(offline)
    RUNNER_BUSY_COUNT.labels(scope, repository).set(busy)
    RUNNER_IDLE.labels(scope, repository).set(idle)

    logger.info(
        "%s/%s: total=%d online=%d offline=%d busy=%d idle=%d",
        scope, repository, total, online, offline, busy, idle
    )


def collect():

    try:
        # Organization-level runners
        if GITHUB_ORG:
            endpoint = f"{GITHUB_API}/orgs/{GITHUB_ORG}/actions/runners"

            collect_runners(
                endpoint,
                "organization",
                GITHUB_ORG
            )

        # Repository-level runners
        for repo in filter(None, GITHUB_REPOS.split(",")):
            repo = repo.strip()

            endpoint = (
                f"{GITHUB_API}/repos/{repo}/actions/runners"
            )

            collect_runners(
                endpoint,
                "repository",
                repo
            )

        EXPORTER_UP.set(1)
        LAST_COLLECTION.set(time.time())

    except requests.RequestException as exc:
        logger.exception("GitHub API request failed: %s", exc)
        EXPORTER_UP.set(0)

    except Exception:
        logger.exception("Unexpected collection error")
        EXPORTER_UP.set(0)


def main():

    logger.info(
        "Starting GitHub Actions Runner Exporter on port %d",
        EXPORTER_PORT
    )

    start_http_server(EXPORTER_PORT)

    while True:
        collect()
        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()

