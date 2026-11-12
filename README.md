# Introduction
This is a Python Prometheus exporter that connects to the GitHub REST API and collects the status of GitHub Actions self-hosted runners.
It supports:

* Organization-level runners.
* Repository-level runners.
* Online/offline status.
* Busy/idle status.
* Runner counts and availability.
* Runner OS, labels and version.
* Prometheus metrics.
* Automatic polling of GitHub.
* API error and exporter health monitoring.

# Install
Clone this reepo
```
git clone https://github.com/rezizter/github-runner-prometheus-exporter.git
cd github-runner-prometheus-exporter
```

Install the required packages.
```
python3 -m venv venv
source venv/bin/activate

pip install -r requirements.txt
```

# Configure GitHub authentication
Create a GitHub Personal Access Token (fine-grained).

Required permissions:
* Organization runners: Self-hosted runners: Read-only
* Repository runners: Administration: Read-only
The token must also have the necessary organization/repository access. See the [GitHub runner API documentation](https://docs.github.com/en/rest/actions/self-hosted-runners?apiVersion=2026-03-10) .

Set environment variables:
```
export GITHUB_TOKEN="github_pat_xxxxxxxxx"
export GITHUB_ORG="my-organization"

# Optional: monitor specific repositories
export GITHUB_REPOS="my-organization/repo1,my-organization/repo2"

export POLL_INTERVAL=60
export EXPORTER_PORT=9877
```

Start the exporter:
```
python github_runner_exporter.py
```
