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
