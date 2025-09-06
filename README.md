# Ubiquity Thread Route Updater

A Go daemon that continuously monitors your network for Matter devices and Thread Border Routers using mDNS discovery, providing real-time routing information for Ubiquity networks.

## Features

- **🔄 Continuous Monitoring**: Runs as a daemon, continuously discovering devices and routers
- **📡 mDNS Discovery**: Automatically discovers Matter devices and Thread Border Routers on your network
- **🌐 IPv6 Support**: Focuses on IPv6 addresses for Thread networking
- **📊 Live Dashboard**: Real-time display of discovered devices and current routes
- **🔧 CIDR Calculation**: Automatically calculates /64 CIDR blocks for discovered networks
- **🛣️ Route Generation**: Generates routing entries for Thread networks that need routing
- **🎯 Smart Filtering**: Excludes main network CIDRs where Thread Border Routers are already located
- **🔗 Ubiquity Integration**: Automatically updates static routes on Ubiquity routers via API

## Installation

1. Clone the repository:

```bash
git clone <repository-url>
cd ubiquity-thread-route-updater
```

2. Install dependencies:

```bash
go mod tidy
```

3. Build the application:

```bash
go build -o thread-route-updater .
```

4. Configure Ubiquity router integration (optional):

```bash
# Copy the example configuration
cp config.env.example config.env

# Edit the configuration with your router settings
nano config.env
```

5. Set environment variables (or use config.env):

```bash
export UBIQUITY_ENABLED=true
export UBIQUITY_ROUTER_HOSTNAME=unifi.local
export UBIQUITY_USERNAME=ubnt
export UBIQUITY_PASSWORD=ubnt
export UBIQUITY_INSECURE_SSL=true
```

## Usage

### Run the Daemon

```bash
./thread-route-updater
```

This will start the daemon with a live dashboard showing:

- Current Matter devices discovered
- Thread Border Routers found
- Real-time routing information
- Last update timestamp

## Daemon Features

### Live Dashboard

The daemon provides a real-time dashboard that updates every 5 seconds, showing:

```
🔍 Thread Route Updater Daemon - Live Status
==================================================
📅 Last Update: 19:55:30

📱 Matter Devices: 19
  • A0C30DA7DF03460A-000000000001B669._matter._tcp.local. -> 2a02:8109:aa22:4181:548a:5ff:fe83:c65e
  • 284F0E33172DF8CA-117ED04504CCE000._matter._tcp.local. -> 2a02:8109:aa22:4181:1ac2:3cff:fe41:4a02
  ... and 17 more

🌐 Thread Border Routers: 3
  • Bathroom\ HomePod -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (2a02:8109:aa22:4181::/64)
  • Kitchen\ HomePod -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (2a02:8109:aa22:4181::/64)
  • Living\ Room\ Apple\ TV\ \(4\) -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (2a02:8109:aa22:4181::/64)

🛣️  Current Routes:
  fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Bathroom\ HomePod)
  fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Kitchen\ HomePod)
  fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Living\ Room\ Apple\ TV\ \(4\))

🔄 Monitoring... (Next update in 5s)
```

### Continuous Monitoring

- **Matter Devices**: Scanned every 30 seconds
- **Thread Border Routers**: Scanned every 30 seconds
- **Dashboard Updates**: Refreshed every 5 seconds
- **Automatic Recovery**: Handles network interruptions gracefully

## Output Format

The daemon outputs routing information in the following format:

```
IPV6_CIDR_BLOCK -> THREAD_BORDER_ROUTER_IPV6 (ROUTER_NAME)
```

Example output:

```
fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Bathroom\ HomePod)
fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Kitchen\ HomePod)
fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Living\ Room\ Apple\ TV\ \(4\))
```

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
8. **📱 Live Updates**: Dashboard refreshes every 5 seconds with current information

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

### Example Output

```
🔄 Updating Ubiquity router static routes...
✅ Added route: fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:cb5:3e92:7a5c:16d6 (Thread route via Bathroom HomePod)
✅ Added route: fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:6c:3a9c:4754:7613 (Thread route via Kitchen HomePod)
✅ Added route: fd6d:56e7:a3c6::/64 -> 2a02:8109:aa22:4181:1ce1:5daf:ce99:f16c (Thread route via Living Room Apple TV (4))
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

- **No devices found**: Ensure your network has Matter devices and Thread Border Routers
- **mDNS issues**: Check that mDNS is working on your network
- **IPv6 issues**: Verify that your devices have IPv6 addresses
- **Permission issues**: Ensure the daemon has network access permissions
- **Build issues**: Make sure you have Go 1.21+ installed

## License

MIT License
