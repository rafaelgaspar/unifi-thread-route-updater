# Ubiquity Thread Route Updater

> **⚠️ AI Development Disclaimer**: This project was heavily developed with the assistance of AI tools (Claude, GitHub Copilot, etc.). While the code has been tested and is functional, please review it thoroughly before deploying in production environments.

A Go daemon that continuously monitors your network for Matter devices and Thread Border Routers using mDNS discovery, automatically managing static routes on Ubiquity routers. This is a **personal pet project** designed for homelab environments.

## 🚀 Features

- **🐳 Docker Support**: Multi-architecture Docker images with security best practices
- **☸️ Kubernetes Ready**: Complete Helm chart for easy deployment
- **📦 OCI Registry**: Published to GitHub Container Registry (GHCR)
- **🔄 CI/CD Pipeline**: Automated testing, building, and publishing
- **📊 Structured Logging**: Configurable log levels (DEBUG, INFO, WARN, ERROR)
- **🔒 Security**: Non-root containers, vulnerability scanning, and secure defaults
- **📈 Monitoring**: Health checks and readiness probes

## Features

- **🔄 Continuous Monitoring**: Runs as a daemon, continuously discovering devices and routers
- **📡 mDNS Discovery**: Automatically discovers Matter devices and Thread Border Routers on your network
- **🌐 IPv6 Support**: Focuses on IPv6 addresses for Thread networking
- **📊 Structured Logging**: Configurable log levels (DEBUG, INFO, WARN, ERROR) with proper daemon output
- **🔧 CIDR Calculation**: Automatically calculates /64 CIDR blocks for discovered networks
- **🛣️ Route Generation**: Generates routing entries for Thread networks that need routing
- **🎯 Smart Filtering**: Excludes main network CIDRs where Thread Border Routers are already located
- **🔗 Ubiquity Integration**: Automatically updates static routes on Ubiquity routers via API
- **⏰ Grace Period Management**: Configurable grace period for route removal with detailed status reporting
- **🔍 Route Lifecycle Tracking**: Shows both detected and configured routes with deletion status

## 🚀 Quick Start

### Option 1: Kubernetes Deployment (Recommended)

1. Add the Helm repository:

```bash
helm repo add thread-route-updater oci://ghcr.io/rafaelgaspar/thread-route-updater
helm repo update
```

2. Create a values file:

```yaml
# values.yaml
config:
  logLevel: "INFO"
  ubiquiti:
    enabled: true
    hostname: "unifi.local.rafaelgaspar.xyz"
    username: "thread-route-updater"
    insecureSSL: false

secrets:
  ubiquitiPassword: "your-secure-password"
```

3. Deploy to Kubernetes:

```bash
helm install thread-route-updater thread-route-updater/thread-route-updater \
  --namespace thread-route-updater \
  --create-namespace \
  --values values.yaml
```

### Option 2: Docker Deployment

```bash
docker run -d \
  --name thread-route-updater \
  -e LOG_LEVEL=INFO \
  -e UBIQUITY_ROUTER_HOSTNAME="unifi.local.rafaelgaspar.xyz" \
  -e UBIQUITY_ROUTER_USERNAME="thread-route-updater" \
  -e UBIQUITY_ROUTER_PASSWORD="your-password" \
  -e UBIQUITY_ROUTER_ENABLED=true \
  ghcr.io/rafaelgaspar/thread-route-updater:latest
```

### Option 3: Local Development

1. Clone the repository:

```bash
git clone https://github.com/rafaelgaspar/ubiquity-thread-route-updater.git
cd ubiquity-thread-route-updater
```

2. Build and run:

```bash
go mod tidy
go build -o thread-route-updater .
./thread-route-updater
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level (DEBUG, INFO, WARN, ERROR) | `INFO` |
| `UBIQUITY_ROUTER_HOSTNAME` | Ubiquiti router hostname | Required |
| `UBIQUITY_ROUTER_USERNAME` | Ubiquiti router username | Required |
| `UBIQUITY_ROUTER_PASSWORD` | Ubiquiti router password | Required |
| `UBIQUITY_ROUTER_ENABLED` | Enable Ubiquiti integration | `true` |
| `UBIQUITY_ROUTER_INSECURE_SSL` | Skip SSL verification | `false` |
| `ROUTE_GRACE_PERIOD` | Grace period before removing routes (e.g., `10m`, `30m`, `1h`) | `10m` |

### Log Level Configuration

The `LOG_LEVEL` environment variable controls the verbosity of the daemon output:

- **DEBUG**: Shows all discovery details, device lists, route information, and grace period status
- **INFO**: Shows status updates, route changes, and important notifications (recommended for production)
- **WARN**: Shows only warnings and errors
- **ERROR**: Shows only critical errors

Example usage:
```bash
LOG_LEVEL=DEBUG ./thread-route-updater  # Verbose output for debugging
LOG_LEVEL=INFO ./thread-route-updater   # Normal operation (default)
LOG_LEVEL=WARN ./thread-route-updater   # Quiet operation
```

## 🏗️ Deployment

### Kubernetes with Helm

The application can be deployed in Kubernetes clusters using Helm charts.

#### Features

- **Multi-architecture support** (AMD64, ARM64)
- **Security hardened** (non-root containers, read-only filesystem)
- **Health checks** and readiness probes
- **Configurable resource limits**
- **Secret management** for sensitive data (create new or use existing secrets)
- **Service account** with minimal permissions

#### Advanced Configuration

```yaml
# values.yaml
replicaCount: 1

image:
  repository: ghcr.io/rafaelgaspar/thread-route-updater
  tag: "latest"
  pullPolicy: IfNotPresent

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi

config:
  logLevel: "INFO"
  ubiquiti:
    enabled: true
    hostname: "unifi.local.rafaelgaspar.xyz"
    username: "thread-route-updater"
    insecureSSL: false

secrets:
  ubiquitiPassword: "your-secure-password"

# Enable monitoring
serviceMonitor:
  enabled: true
  interval: 30s
```

#### Using Existing Secrets

For advanced secret management (e.g., External Secrets Operator, Sealed Secrets), you can use an existing secret instead of creating a new one:

```yaml
# values.yaml
secrets:
  create: false
  secretName: "my-existing-secret"
```

### CI/CD Pipeline

The project includes a complete CI/CD pipeline with:

- **Automated testing** on every push and PR
- **Multi-architecture Docker builds** (AMD64, ARM64)
- **Security scanning** with Trivy
- **Helm chart packaging** and OCI publishing
- **Automated releases** with binary artifacts
- **Dependency updates** with Dependabot

### Monitoring and Observability

- **Structured logging** with configurable levels
- **Health checks** for container orchestration
- **Prometheus metrics** (when serviceMonitor is enabled)
- **Security scanning** in CI/CD pipeline

## Usage

The daemon runs continuously in the background, providing structured logging output with configurable severity levels. It monitors your network for Matter devices and Thread Border Routers, automatically managing routes on your Ubiquity router.

### Log Output Example

```
2025/09/07 00:58:38 [INFO] Thread Route Updater daemon starting...
2025/09/07 00:58:38 [INFO] Status update: 13 Matter devices, 3 Thread Border Routers, 2 routes detected
2025/09/07 00:58:38 [INFO] Detected routes: 2 routes
2025/09/07 00:58:38 [DEBUG] Detected route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Kitchen HomePod)
2025/09/07 00:58:38 [DEBUG] Detected route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Bathroom HomePod)
2025/09/07 00:58:38 [INFO] Configured routes: 3 Thread routes in Ubiquity router
2025/09/07 00:58:38 [DEBUG] Configured route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Thread route via Bathroom HomePod)
2025/09/07 00:58:38 [DEBUG] Configured route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Thread route via Kitchen HomePod)
2025/09/07 00:58:38 [INFO] Route marked for deletion: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV) - will be removed in 7m
```

## Daemon Features

### Structured Logging

The daemon provides structured logging with configurable severity levels:

- **DEBUG**: Detailed discovery information, device lists, route details
- **INFO**: Status updates, route changes, grace period notifications
- **WARN**: Authentication issues, overdue route deletions, configuration problems
- **ERROR**: Critical failures, network issues, API errors

### Continuous Monitoring

- **Matter Devices**: Scanned every 30 seconds
- **Thread Border Routers**: Scanned every 30 seconds
- **Status Updates**: Reported every 30 seconds
- **Automatic Recovery**: Handles network interruptions gracefully
- **Route Grace Period**: Routes are not immediately removed when devices go offline, preventing temporary route churn

### Route Grace Period

To prevent unnecessary route churn when mDNS announcements are infrequent, the application implements a configurable grace period:

- **Default**: 10 minutes (`ROUTE_GRACE_PERIOD=10m`)
- **Behavior**: Routes are only removed after being absent for the full grace period
- **Benefits**: Prevents temporary route deletion when devices briefly go offline
- **Configurable**: Set via `ROUTE_GRACE_PERIOD` environment variable (e.g., `30m`, `2h`, `1h30m`)

#### Grace Period Status Messages

The daemon provides detailed status information about routes in different states:

**Routes within grace period:**
```
[INFO] Route marked for deletion: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV) - will be removed in 7m
```

**Routes overdue for deletion:**
```
[WARN] Route overdue for deletion: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV) - grace period expired
```

**Routes never seen before (gets full grace period):**
```
[INFO] Route marked for deletion: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV) - will be removed in 10m (grace period)
```

Example scenarios:

- Device goes offline for 5 minutes → Route kept (within grace period)
- Device goes offline for 15 minutes → Route removed (exceeds grace period)
- Device comes back online → Route immediately restored

## Output Format

The daemon outputs structured logging with different severity levels. Route information is displayed in the following format:

```
IPV6_CIDR_BLOCK -> THREAD_BORDER_ROUTER_IPV6 (ROUTER_NAME)
```

Example log output:

```
2025/09/07 00:58:38 [DEBUG] Detected route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Kitchen HomePod)
2025/09/07 00:58:38 [DEBUG] Detected route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Bathroom HomePod)
2025/09/07 00:58:38 [DEBUG] Configured route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Thread route via Bathroom HomePod)
```

Note: Router names are automatically unescaped from mDNS format (e.g., `Living\ Room\ Apple\ TV\ \(4\)` becomes `Living Room Apple TV (4)`).

## How It Works

1. **🔄 Continuous Monitoring**: Two separate goroutines continuously monitor for devices and routers
2. **📡 mDNS Discovery**: Uses the `github.com/grandcat/zeroconf` library to browse for mDNS services
3. **🎯 Service Types**:
   - `_matter._tcp` for Matter devices
   - `_meshcop._udp` for Thread Border Routers
4. **🌐 IPv6 Processing**: Extracts real IPv6 addresses (not IPv4-mapped)
5. **📊 CIDR Calculation**: Calculates /64 network prefixes from IPv6 addresses
6. **🛣️ Route Generation**: Creates routes only for Thread networks that need routing (excludes main network)
7. **🔗 Ubiquity Integration**: Automatically updates static routes on Ubiquity routers via REST API
8. **📊 Structured Logging**: Provides detailed status updates every 30 seconds with configurable log levels
9. **⏰ Grace Period Management**: Tracks route lifecycle and provides detailed deletion status

## Ubiquity Router Integration

The daemon can automatically update static routes on your Ubiquity router when Thread networks are discovered. This ensures that your router always has the correct routes to reach Thread devices.

### Configuration

Set these environment variables to enable Ubiquity integration:

| Variable | Description | Default |
|----------|-------------|---------|
| `UBIQUITY_ENABLED` | Enable Ubiquity integration | `false` |
| `UBIQUITY_ROUTER_HOSTNAME` | Router hostname | `unifi.local` |
| `UBIQUITY_USERNAME` | Router username | `ubnt` |
| `UBIQUITY_PASSWORD` | Router password | `ubnt` |
| `UBIQUITY_INSECURE_SSL` | Allow self-signed certificates | `false` |

### How It Works

1. **Route Discovery**: Daemon discovers Thread networks and generates routes
2. **API Communication**: Connects to Ubiquity router via REST API
3. **Route Comparison**: Compares current router routes with desired routes
4. **Automatic Updates**: Adds new routes and removes old Thread routes
5. **Smart Management**: Only manages routes created by the daemon (marked with "Thread route via")

### Example Log Output

```
2025/09/07 00:58:38 [INFO] Updating Ubiquity router static routes...
2025/09/07 00:58:38 [INFO] Added route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Thread route via Bathroom HomePod)
2025/09/07 00:58:38 [INFO] Added route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Thread route via Kitchen HomePod)
2025/09/07 00:58:38 [INFO] Removed route: fd04:29fc:597e:1::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV)
```

## Development Commands

| Command | Description |
|---------|-------------|
| `go build -o thread-route-updater .` | Build the application |
| `go run .` | Run in development mode |
| `go test ./...` | Run tests |
| `go mod tidy` | Install/update dependencies |
| `go clean` | Clean build artifacts |

## Dependencies

- Go 1.21+
- `github.com/grandcat/zeroconf` - mDNS service discovery

## Troubleshooting

### Common Issues

- **No devices found**: Ensure your network has Matter devices and Thread Border Routers
- **mDNS issues**: Check that mDNS is working on your network
- **IPv6 issues**: Verify that your devices have IPv6 addresses
- **Permission issues**: Ensure the daemon has network access permissions
- **Build issues**: Make sure you have Go 1.21+ installed

### Log Analysis

Use different log levels to troubleshoot issues:

```bash
# Debug mode - shows all discovery details
LOG_LEVEL=DEBUG ./thread-route-updater

# Check for authentication issues
LOG_LEVEL=WARN ./thread-route-updater

# Monitor route lifecycle
LOG_LEVEL=INFO ./thread-route-updater
```

### Common Log Messages

- **`[WARN] Failed to get configured routes from Ubiquity router: API request failed with status 401`**: Authentication issue - check credentials
- **`[INFO] Route marked for deletion: ... - will be removed in Xm`**: Normal grace period behavior
- **`[WARN] Route overdue for deletion: ... - grace period expired`**: Route should be removed but may be stuck
- **`[DEBUG] No valid session tokens for route status check`**: Normal when session expires, will re-authenticate

## 🤖 About This Project

### Pet Project Disclaimer

This is a **personal pet project** created for my homelab environment. It's primarily intended for:

- **Learning and experimentation** with Go, Kubernetes, and networking
- **Personal use** in homelab environments
- **Demonstration** of modern DevOps practices

### AI-Assisted Development

This project was **heavily developed with AI assistance**, including:

- **Claude (Anthropic)** - Primary development assistance, code generation, and debugging
- **GitHub Copilot** - Code completion and suggestions
- **ChatGPT** - Documentation and troubleshooting assistance

While the code has been tested and is functional, please:

- **Review thoroughly** before deploying
- **Test in your environment** before relying on it
- **Understand the code** before making modifications

### Contributing

Contributions are welcome! This project serves as a learning exercise, so feel free to:

- **Report issues** and bugs
- **Suggest improvements** and features
- **Submit pull requests** for fixes or enhancements
- **Share your deployment experiences**

### Roadmap

Future improvements might include:

- **Prometheus metrics** integration
- **Webhook notifications** for route changes
- **Multiple router support**
- **Configuration validation**
- **Better error handling** and recovery

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- **AI Tools** - Claude, GitHub Copilot, and ChatGPT for development assistance
- **Open Source Community** - For the excellent Go libraries and tools
- **Ubiquiti** - For their networking equipment and API documentation
