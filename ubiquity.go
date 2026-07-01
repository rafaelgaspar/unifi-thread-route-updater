package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// updateUbiquityRoutes updates the Ubiquity router with the current routes
func updateUbiquityRoutes(state *DaemonState, routes []Route) {
	if !state.UbiquityConfig.Enabled {
		return
	}

	state.routeSyncMu.Lock()
	defer state.routeSyncMu.Unlock()

	logInfo("UniFi: syncing static routes...")

	if !state.UbiquityConfig.hasValidSession() {
		logInfo("UniFi: authenticating...")
		if err := loginToUbiquity(&state.UbiquityConfig); err != nil {
			logError("UniFi: login failed: %v", err)
			return
		}
	} else {
		logDebug("UniFi: reusing session (age %s)", formatDuration(time.Since(state.UbiquityConfig.LastLogin)))
	}

	currentRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	if err != nil {
		logError("UniFi: failed to get current routes: %v", err)
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "AUTHENTICATION_FAILED_LIMIT_REACHED") {
			logWarn("UniFi: rate limit reached, skipping")
			state.UbiquityConfig.clearSession()
			return
		}
		state.UbiquityConfig.clearSession()
		if err = loginToUbiquity(&state.UbiquityConfig); err != nil {
			logError("UniFi: re-login failed: %v", err)
			return
		}
		currentRoutes, err = getUbiquityStaticRoutes(state.UbiquityConfig)
		if err != nil {
			logError("UniFi: failed to get routes after re-login: %v", err)
			return
		}
	}

	// Discover gateway device MAC from existing routes if not already known.
	if state.UbiquityConfig.GatewayDevice == "" {
		for _, r := range currentRoutes {
			if r.GatewayDevice != "" {
				state.UbiquityConfig.GatewayDevice = r.GatewayDevice
				logDebug("UniFi: discovered gateway device %s", r.GatewayDevice)
				break
			}
		}
	}
	if state.UbiquityConfig.GatewayDevice == "" {
		if mac, err := fetchGatewayDeviceMAC(state.UbiquityConfig); err != nil {
			logWarn("UniFi: could not determine gateway device: %v", err)
		} else {
			state.UbiquityConfig.GatewayDevice = mac
			logDebug("UniFi: discovered gateway device %s via device API", mac)
		}
	}

	desiredRoutes := convertToUbiquityRoutes(routes, state.UbiquityConfig.GatewayDevice)

	state.mu.Lock()
	routeUpdateTime := time.Now()
	for _, route := range desiredRoutes {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		state.RouteLastSeen[key] = routeUpdateTime
	}
	routesToAdd, routesToRemove := compareRoutesWithGracePeriod(currentRoutes, desiredRoutes, state.RouteLastSeen, state.UbiquityConfig.RouteGracePeriod)
	state.mu.Unlock()

	distances := newDistanceAllocator(currentRoutes)
	distances.assign(routesToAdd)

	if len(routesToAdd) > 0 || len(routesToRemove) > 0 {
		logInfo("UniFi: route changes +%d -%d", len(routesToAdd), len(routesToRemove))
	}

	if len(routesToAdd) > 0 {
		time.Sleep(2 * time.Second)
	}

	for _, route := range routesToRemove {
		logInfo("UniFi: deleting route %s -> %s (id=%s)...",
			route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID)
		if err := deleteUbiquityStaticRoute(state.UbiquityConfig, route.ID); err != nil {
			logError("UniFi: delete failed %s (id=%s): %v", route.StaticRouteNetwork, route.ID, err)
			if strings.Contains(err.Error(), "IdInvalid") {
				logWarn("UniFi: route id invalid, already deleted")
				key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
				state.mu.Lock()
				delete(state.RouteLastSeen, key)
				delete(state.AddedRoutes, key)
				state.mu.Unlock()
			}
		} else {
			logInfo("UniFi: deleted route %s -> %s", route.StaticRouteNetwork, route.StaticRouteNexthop)
			key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
			state.mu.Lock()
			delete(state.AddedRoutes, key)
			state.mu.Unlock()
		}
	}

	for i := range routesToAdd {
		route := routesToAdd[i]
		for attempt := 0; attempt < 5; attempt++ {
			err := addUbiquityStaticRoute(state.UbiquityConfig, route)
			if err == nil {
				logInfo("UniFi: added route %s -> %s (%s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
				key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
				state.mu.Lock()
				state.AddedRoutes[key] = true
				state.mu.Unlock()
				break
			}
			if strings.Contains(err.Error(), "DestinationNetworkExisted") && attempt < 4 {
				prefix := route.StaticRouteNetwork
				distances.markUsed(prefix, route.StaticRouteDistance)
				next, ok := distances.nextFree(prefix)
				for !ok {
					distances.count[prefix]++
					next, ok = distances.nextFree(prefix)
				}
				route.StaticRouteDistance = next
				routesToAdd[i].StaticRouteDistance = next
				distances.markUsed(prefix, next)
				logWarn("UniFi: distance collision for %s, retrying with distance %d",
					prefix, next)
				continue
			}
			logError("UniFi: add failed %s: %v", route.StaticRouteNetwork, err)
			break
		}
	}

	if len(routesToAdd) == 0 && len(routesToRemove) == 0 {
		logDebug("UniFi: routes up to date")
	}
}

// getUbiquityStaticRoutes retrieves current static routes from the router
func getUbiquityStaticRoutes(config UbiquityConfig) ([]UbiquityStaticRoute, error) {
	client := createHTTPClient(config)
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing", config.APIBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, config)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp UbiquityAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Meta.RC != "ok" {
		return nil, fmt.Errorf("API returned error: %s", apiResp.Meta.RC)
	}

	return apiResp.Data, nil
}

// addUbiquityStaticRoute adds a new static route to the router
func addUbiquityStaticRoute(config UbiquityConfig, route UbiquityStaticRoute) error {
	client := createHTTPClient(config)
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing", config.APIBaseURL)

	jsonData, err := json.Marshal(route)
	if err != nil {
		return err
	}
	logDebug("UniFi: add route payload: %s", string(jsonData))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	applyAuth(req, config)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logDebug("UniFi: add route response: status=%d body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// deleteUbiquityStaticRoute deletes a static route from the router
func deleteUbiquityStaticRoute(config UbiquityConfig, routeID string) error {
	client := createHTTPClient(config)
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/%s", config.APIBaseURL, routeID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "thread-route-updater/1.0")
	applyAuth(req, config)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// applyAuth sets the authentication headers and cookie on a request.
func applyAuth(req *http.Request, config UbiquityConfig) {
	req.Header.Set("Content-Type", "application/json")
	if config.SessionCookie != "" {
		req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
		req.AddCookie(&http.Cookie{Name: "TOKEN", Value: config.SessionCookie})
	}
	if config.CSRFToken != "" {
		req.Header.Set("X-CSRF-Token", config.CSRFToken)
	}
}

// closeBody drains and closes the response body, logging any error.
func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		logWarn("UniFi: failed to close response: %v", err)
	}
}

// createHTTPClient creates an HTTP client with appropriate settings
func createHTTPClient(config UbiquityConfig) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureSSL},
		},
		Timeout: 30 * time.Second,
	}
}

// convertToUbiquityRoutes converts our Route format to Ubiquity format.
// Distance is left as 0 for new routes; callers should call assignRouteDistances
// after fetching current routes from UniFi to avoid metric collisions.
func convertToUbiquityRoutes(routes []Route, gatewayDevice string) []UbiquityStaticRoute {
	var ubiquityRoutes []UbiquityStaticRoute
	for _, route := range routes {
		cleanRouterName := strings.ReplaceAll(route.RouterName, "\\", "")
		ubiquityRoutes = append(ubiquityRoutes, UbiquityStaticRoute{
			Enabled:            true,
			Name:               fmt.Sprintf("Thread route via %s", cleanRouterName),
			Type:               "static-route",
			StaticRouteNexthop: route.ThreadRouterIPv6,
			StaticRouteNetwork: route.CIDR,
			StaticRouteType:    "nexthop-route",
			GatewayType:        "default",
			GatewayDevice:      gatewayDevice,
		})
	}
	return ubiquityRoutes
}

// distanceAllocator picks the lowest unused distance in 1..N per destination prefix,
// where N is the total route count for that prefix (existing + pending adds).
type distanceAllocator struct {
	used  map[string]map[int]bool
	count map[string]int
}

func newDistanceAllocator(current []UbiquityStaticRoute) *distanceAllocator {
	a := &distanceAllocator{
		used:  make(map[string]map[int]bool),
		count: make(map[string]int),
	}
	zeroDist := make(map[string]int)
	for _, r := range current {
		prefix := r.StaticRouteNetwork
		a.count[prefix]++
		if a.used[prefix] == nil {
			a.used[prefix] = make(map[int]bool)
		}
		if r.StaticRouteDistance > 0 {
			a.used[prefix][r.StaticRouteDistance] = true
		} else {
			zeroDist[prefix]++
		}
	}
	// GET often omits static-route_distance; reserve the lowest slots for those routes.
	for prefix, zeros := range zeroDist {
		for z := 0; z < zeros; z++ {
			if d, ok := a.nextFree(prefix); ok {
				a.markUsed(prefix, d)
			}
		}
	}
	return a
}

func (a *distanceAllocator) markUsed(prefix string, distance int) {
	if a.used[prefix] == nil {
		a.used[prefix] = make(map[int]bool)
	}
	a.used[prefix][distance] = true
}

// nextFree returns the lowest unused distance in 1..count[prefix].
func (a *distanceAllocator) nextFree(prefix string) (int, bool) {
	for d := 1; d <= a.count[prefix]; d++ {
		if used := a.used[prefix]; used == nil || !used[d] {
			return d, true
		}
	}
	return 0, false
}

func (a *distanceAllocator) assign(toAdd []UbiquityStaticRoute) {
	for i := range toAdd {
		prefix := toAdd[i].StaticRouteNetwork
		a.count[prefix]++
		d, ok := a.nextFree(prefix)
		if !ok {
			// Should not happen: N routes should always have a free slot in 1..N.
			d = a.count[prefix]
		}
		a.markUsed(prefix, d)
		toAdd[i].StaticRouteDistance = d
	}
}

// compareRoutesWithGracePeriod compares current and desired routes with grace period consideration
func compareRoutesWithGracePeriod(current, desired []UbiquityStaticRoute, routeLastSeen map[string]time.Time, gracePeriod time.Duration) ([]UbiquityStaticRoute, []UbiquityStaticRoute) {
	var toAdd, toRemove []UbiquityStaticRoute
	now := time.Now()

	desiredMap := make(map[string]UbiquityStaticRoute, len(desired))
	for _, route := range desired {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		desiredMap[key] = route
	}

	for _, cur := range current {
		key := fmt.Sprintf("%s->%s", cur.StaticRouteNetwork, cur.StaticRouteNexthop)
		if _, exists := desiredMap[key]; exists {
			continue
		}
		if !strings.Contains(cur.Name, "Thread route via") {
			continue
		}
		if lastSeen, seen := routeLastSeen[key]; seen {
			if now.Sub(lastSeen) < gracePeriod {
				continue // within grace period
			}
		} else {
			logDebug("UniFi: route %s -> %s not in detected routes, grace period started",
				cur.StaticRouteNetwork, cur.StaticRouteNexthop)
			routeLastSeen[key] = now
			continue
		}
		toRemove = append(toRemove, cur)
	}

	currentMap := make(map[string]bool, len(current))
	for _, route := range current {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		currentMap[key] = true
	}
	for _, des := range desired {
		key := fmt.Sprintf("%s->%s", des.StaticRouteNetwork, des.StaticRouteNexthop)
		if !currentMap[key] {
			toAdd = append(toAdd, des)
		}
	}

	return toAdd, toRemove
}

// fetchGatewayDeviceMAC retrieves the gateway device MAC from /stat/device (type=udm).
func fetchGatewayDeviceMAC(config UbiquityConfig) (string, error) {
	client := createHTTPClient(config)
	url := fmt.Sprintf("%s/proxy/network/api/s/default/stat/device", config.APIBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	applyAuth(req, config)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeBody(resp)

	var result struct {
		Data []struct {
			Type string `json:"type"`
			MAC  string `json:"mac"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, d := range result.Data {
		if d.Type == "udm" && d.MAC != "" {
			return d.MAC, nil
		}
	}
	return "", fmt.Errorf("gateway device (type=udm) not found in /stat/device response")
}

// loginToUbiquity authenticates with the Ubiquity router and gets a session token
func loginToUbiquity(config *UbiquityConfig) error {
	client := createHTTPClient(*config)
	url := fmt.Sprintf("%s/api/auth/login", config.APIBaseURL)

	jsonData, err := json.Marshal(UbiquityLoginRequest{
		Username: config.Username,
		Password: config.Password,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var loginResp UbiquityLoginResponse
	if err := json.Unmarshal(body, &loginResp); err == nil && loginResp.Meta.RC == "ok" {
		// standard format
	} else {
		var userProfile map[string]interface{}
		if err := json.Unmarshal(body, &userProfile); err != nil {
			return fmt.Errorf("failed to parse login response: %v, body: %s", err, string(body))
		}
		if username, ok := userProfile["username"].(string); !ok || username != config.Username {
			return fmt.Errorf("login failed: invalid user profile, body: %s", string(body))
		}
	}

	if csrfToken := resp.Header.Get("X-CSRF-Token"); csrfToken != "" {
		config.CSRFToken = csrfToken
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "TOKEN" || cookie.Name == "unifises" {
			config.SessionCookie = cookie.Value
		}
	}

	config.LastLogin = time.Now()
	return nil
}
