# Contributing

Thanks for considering a contribution to `unifi-thread-route-updater`.

## Dev setup

```bash
git clone https://github.com/rafaelgaspar/unifi-thread-route-updater.git
cd unifi-thread-route-updater
go mod download
go build -o thread-route-updater .
go test -v ./...
```

Run the linter locally with the same tool CI uses:

```bash
golangci-lint run
```

If you're touching `chart/`, also run:

```bash
helm lint chart/unifi-thread-route-updater
ct lint --all --config ct.yaml
```

## Code style

- Keep new code consistent with the existing style rather than introducing a new pattern.
- No comments explaining *what* code does; only *why*, and only when genuinely non-obvious (a workaround, an invariant, a subtle constraint).
- `go vet`/`golangci-lint` must pass cleanly — CI enforces this on every push/PR.

## Workflow

1. Fork the repo and branch off `main`.
2. Keep PRs scoped to one logical change — avoid bundling unrelated fixes/features.
3. Make sure `go test -v ./...` and `golangci-lint run` pass locally before opening the PR (and `ct lint` if you touched the chart). CI runs the same checks and must be green before merge.
4. Update the README/chart values docs alongside any user-facing change (new env var, new chart value, new CLI flag).

## Releasing (maintainers)

Releases are cut by pushing a `vX.Y.Z` tag from `main` (matching `chart/unifi-thread-route-updater/Chart.yaml`'s `version`). That triggers `.github/workflows/build-and-release.yml`, which independently:

- Builds and pushes the multi-arch Docker image to `ghcr.io/rafaelgaspar/unifi-thread-route-updater`, tagged with the release version, `{major}.{minor}`, and `latest`.
- Runs a Trivy vulnerability scan against the built image and uploads results to the repo's Security tab.
- Packages the Helm chart, attaches it to the image manifest (`oras attach`), and pushes it as a standalone tagged artifact to `oci://ghcr.io/rafaelgaspar/unifi-thread-route-updater/charts`.
- Cross-compiles native binaries (linux/darwin, amd64/arm64, plus windows/amd64) and publishes a GitHub Release with checksums.

Contributors don't need to do any of this — only a maintainer pushing a release tag triggers it.

## Dependency updates

[Renovate](https://docs.renovatebot.com/) is configured via `renovate.json` and runs on its own schedule via `.github/workflows/renovate.yaml` (a self-hosted workflow run, not the Renovate GitHub App) — no separate app installation needed.
