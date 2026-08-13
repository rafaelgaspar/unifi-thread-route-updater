# AGENTS.md

Instructions for AI coding agents working in this repository.

## What this is

`unifi-thread-route-updater` is a Go daemon that discovers Matter devices and Thread Border Routers via mDNS and manages static routes on Ubiquiti routers. Ships as a Docker image + Helm chart, and as cross-compiled native binaries on each release.

## Layout

```
main.go            # entry point
daemon.go           # main run loop
discovery.go        # mDNS discovery
routes.go           # route calculation/lifecycle
ubiquiti.go         # Ubiquiti router API client
homeassistant.go     # optional Home Assistant integration
config.go, logger.go # config parsing, structured logging
*_test.go           # one test file per source file
chart/unifi-thread-route-updater/  # Helm chart (published as an OCI artifact on release)
Dockerfile           # multi-stage golang:alpine build
.github/workflows/    # build-and-release.yml (CI + tag-triggered release), renovate.yaml, gitleaks.yaml
```

## Build, test, lint

```bash
go mod download
go build -o thread-route-updater .
go test -v ./...
golangci-lint run
```

Chart changes:

```bash
helm lint chart/unifi-thread-route-updater
ct lint --all --config ct.yaml
```

Run all of the above before considering a change complete. CI (`build-and-release.yml`'s `test` and `lint-chart` jobs) runs the same checks on every push/PR to `main`.

## Conventions

- No comments explaining *what* code does — only *why*, and only when genuinely non-obvious (a workaround, an invariant, a subtle constraint). Do not restate the code in prose.
- Don't add abstractions, config flags, or generality beyond what's actually needed.
- This daemon runs continuously and manages live network state (routes on a real router) — treat anything that mutates router state as higher-risk than read-only discovery code; keep the grace-period/lifecycle logic in `routes.go` easy to reason about, since a bug there directly affects network routing.

## Chart conventions

- Generic, vendor-neutral Helm chart — no Kubernetes-distribution-specific resources beyond what's already there (the `NetworkAttachmentDefinition` template is intentional — this app needs Multus VLAN attachment for mDNS on a specific L2 segment — but don't add cluster-specific resources like NetworkPolicy/CiliumNetworkPolicy to the chart itself).
- Chart version stays in lockstep with the release tag — don't bump `chart/unifi-thread-route-updater/Chart.yaml`'s `version` independently of a release.

## Release process

Maintainer-only: push a `vX.Y.Z` tag from `main` to trigger `.github/workflows/build-and-release.yml` (multi-arch Docker image, Trivy scan, Helm chart to GHCR — both attached to the image and as a standalone tagged artifact — and a GitHub Release with cross-compiled binaries). Contributors don't need to think about this — see `CONTRIBUTING.md` for details if you do.

## Commit/PR conventions

No project-specific identity or authorship requirements — normal open-source practice applies: your own name/email as author, clear commit messages, one logical change per PR. See `CONTRIBUTING.md`.
