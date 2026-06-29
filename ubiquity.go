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

	logInfo("Updating Ubiquity router static routes...")

	if !state.UbiquityConfig.hasValidSession() {
		logInfo("No valid session, authenticating...")
		if err := loginToUbiquity(&state.UbiquityConfig); err != nil {
			logError("Failed to login to Ubiquity router: %v", err)
			return
		}
	} else {
		logDebug("Using existing session (age: %s)", formatDuration(time.Since(state.UbiquityConfig.LastLogin)))
	}

	currentRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	if err != nil {
		logError("Failed to get current routes: %v", err)
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "AUTHENTICATION_FAILED_LIMIT_REACHED") {
			logWarn("Rate limit reached, skipping this update cycle...")
			state.UbiquityConfig.clearSession()
			return
		}
		state.UbiquityConfig.clearSession()
		if err = loginToUbiquity(&state.UbiquityConfig); err != nil {
			logError("Failed to re-login to Ubiquity router: %v", err)
			return
		}
		currentRoutes, err = getUbiquityStaticRoutes(state.UbiquityConfig)
		if err != nil {
			logError("Failed to get current routes after re-login: %v", err)
			return
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

	if len(routesToAdd) > 0 || len(routesToRemove) > 0 {
		logInfo("Route changes: +%d routes, -%d routes", len(routesToAdd), len(routesToRemove))
	}

	state.mu.Lock()
	var newRoutesToAdd []UbiquityStaticRoute
	for _, route := range routesToAdd {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		if !state.AddedRoutes[key] {
			newRoutesToAdd = append(newRoutesToAdd, route)
			state.AddedRoutes[key] = true
		}
	}
	state.mu.Unlock()
	routesToAdd = newRoutesToAdd

	if len(routesToAdd) > 0 {
		time.Sleep(2 * time.Second)
	}

	for _, route := range routesToRemove {
		logInfo("Attempting to delete route: %s -> %s (ID: %s)",
			route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID)
		if err := deleteUbiquityStaticRoute(state.UbiquityConfig, route.ID); err != nil {
			logError("Failed to delete route %s (ID: %s): %v", route.StaticRouteNetwork, route.ID, err)
			if strings.Contains(err.Error(), "IdInvalid") {
				logWarn("Route ID invalid, likely already deleted. Removing from tracking.")
				key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
				state.mu.Lock()
				delete(state.RouteLastSeen, key)
				delete(state.AddedRoutes, key)
				state.mu.Unlock()
			}
		} else {
			logInfo("Successfully deleted route: %s -> %s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		}
	}

	for _, route := range routesToAdd {
		if err := addUbiquityStaticRoute(state.UbiquityConfig, route); err != nil {
			logError("Failed to add route %s: %v", route.StaticRouteNetwork, err)
		} else {
			logInfo("Successfully added route: %s -> %s (%s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
		}
	}

	if len(routesToAdd) == 0 && len(routesToRemove) == 0 {
		logDebug("Ubiquity routes are up to date")
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
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/static-route", config.APIBaseURL)

	jsonData, err := json.Marshal(route)
	if err != nil {
		return err
	}

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
		logWarn("Failed to close response body: %v", err)
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

// convertToUbiquityRoutes converts our Route format to Ubiquity format
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
			logDebug("Route %s -> %s never seen before, giving grace period, not removing",
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
