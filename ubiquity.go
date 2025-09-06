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

	fmt.Println("🔄 Updating Ubiquity router static routes...")

	// Check if we have valid session tokens and they're not too old
	// Only re-authenticate if we don't have tokens or they're expired
	currentTime := time.Now().Unix()
	timeSinceLastLogin := currentTime - state.UbiquityConfig.LastLoginTime

	if state.UbiquityConfig.SessionCookie == "" || state.UbiquityConfig.CSRFToken == "" {
		fmt.Println("🔐 No valid session tokens, authenticating...")
		err := loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to login to Ubiquity router: %v\n", err)
			return
		}
	} else if timeSinceLastLogin > 300 { // 5 minutes
		fmt.Printf("🔐 Session tokens expired (%d seconds old), re-authenticating...\n", timeSinceLastLogin)
		err := loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to login to Ubiquity router: %v\n", err)
			return
		}
	} else {
		logDebug("Using existing session tokens (%d seconds old)", timeSinceLastLogin)
	}

	// Get current routes from router
	currentRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	if err != nil {
		fmt.Printf("❌ Failed to get current routes: %v\n", err)
		// If we get a rate limit error, don't try to re-login immediately
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "AUTHENTICATION_FAILED_LIMIT_REACHED") {
			fmt.Println("⚠️ Rate limit reached, skipping this update cycle...")
			// Clear all session tokens to force fresh login next time
			state.UbiquityConfig.SessionToken = ""
			state.UbiquityConfig.SessionCookie = ""
			state.UbiquityConfig.CSRFToken = ""
			return
		}
		// For other auth errors, try to re-login and retry once
		state.UbiquityConfig.SessionToken = ""
		state.UbiquityConfig.SessionCookie = ""
		state.UbiquityConfig.CSRFToken = ""
		err = loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to re-login to Ubiquity router: %v\n", err)
			return
		}
		currentRoutes, err = getUbiquityStaticRoutes(state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to get current routes after re-login: %v\n", err)
			return
		}
	}

	// Convert our routes to Ubiquity format
	desiredRoutes := convertToUbiquityRoutes(routes)

	// Update last seen time for current desired routes
	routeUpdateTime := time.Now()
	for _, route := range desiredRoutes {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		state.RouteLastSeen[key] = routeUpdateTime
	}

	// Find routes to add and remove (with grace period consideration)
	routesToAdd, routesToRemove := compareRoutesWithGracePeriod(currentRoutes, desiredRoutes, state.RouteLastSeen, state.UbiquityConfig.RouteGracePeriod)

	// Show summary if there are changes or if we have routes being tracked
	if len(routesToAdd) > 0 || len(routesToRemove) > 0 || len(state.RouteLastSeen) > 0 {
		fmt.Printf("🔄 Route changes: +%d routes, -%d routes (grace period: %s)\n",
			len(routesToAdd), len(routesToRemove), formatDuration(state.UbiquityConfig.RouteGracePeriod))

		// Show grace period status for tracked routes
		if len(state.RouteLastSeen) > 0 {
			currentTime := time.Now()
			for key, lastSeen := range state.RouteLastSeen {
				timeSinceLastSeen := currentTime.Sub(lastSeen)
				if timeSinceLastSeen < state.UbiquityConfig.RouteGracePeriod {
					remaining := state.UbiquityConfig.RouteGracePeriod - timeSinceLastSeen
					remainingStr := formatDuration(remaining)
					fmt.Printf("⏳ Route %s still within grace period (%s remaining)\n", key, remainingStr)
				}
			}
		}
	}

	// Filter out routes we've already added (in-memory tracking)
	var newRoutesToAdd []UbiquityStaticRoute
	for _, route := range routesToAdd {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		if !state.AddedRoutes[key] {
			newRoutesToAdd = append(newRoutesToAdd, route)
			state.AddedRoutes[key] = true // Mark as added
		}
	}
	routesToAdd = newRoutesToAdd

	// Add a small delay after adding routes to allow them to be indexed
	if len(routesToAdd) > 0 {
		time.Sleep(2 * time.Second)
	}

	// Remove old routes
	for _, route := range routesToRemove {
		fmt.Printf("🗑️  Attempting to delete route: %s -> %s (ID: %s)\n",
			route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID)
		if err := deleteUbiquityStaticRoute(state.UbiquityConfig, route.ID); err != nil {
			fmt.Printf("❌ Failed to delete route %s (ID: %s): %v\n", route.StaticRouteNetwork, route.ID, err)
			// If the route ID is invalid, it might have been manually deleted
			// Remove it from our tracking to prevent repeated attempts
			if strings.Contains(err.Error(), "IdInvalid") {
				fmt.Printf("⚠️  Route ID invalid, likely already deleted. Removing from tracking.\n")
				// Remove from in-memory tracking
				key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
				delete(state.RouteLastSeen, key)
				delete(state.AddedRoutes, key)
			}
		} else {
			fmt.Printf("✅ Deleted route: %s -> %s\n", route.StaticRouteNetwork, route.StaticRouteNexthop)
		}
	}

	// Add new routes
	for _, route := range routesToAdd {
		if err := addUbiquityStaticRoute(state.UbiquityConfig, route); err != nil {
			fmt.Printf("❌ Failed to add route %s: %v\n", route.StaticRouteNetwork, err)
		} else {
			fmt.Printf("✅ Added route: %s -> %s (%s)\n", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
		}
	}

	if len(routesToAdd) == 0 && len(routesToRemove) == 0 {
		fmt.Println("✅ Ubiquity routes are up to date")
	}
}

// getUbiquityStaticRoutes retrieves current static routes from the router
func getUbiquityStaticRoutes(config UbiquityConfig) ([]UbiquityStaticRoute, error) {
	client := createHTTPClient(config)

	// Use the correct endpoint for reading routes
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing", config.APIBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add session authentication
	req.Header.Set("Content-Type", "application/json")

	// Use session cookie as Authorization header
	if config.SessionCookie != "" {
		req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
	}

	if config.CSRFToken != "" {
		req.Header.Set("X-CSRF-Token", config.CSRFToken)
	}

	if config.SessionCookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  "TOKEN",
			Value: config.SessionCookie,
		})
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

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

	// Try the UDM Pro/UCG Max endpoint first
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/static-route", config.APIBaseURL)

	jsonData, err := json.Marshal(route)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Add session authentication
	req.Header.Set("Content-Type", "application/json")

	// Use session cookie as Authorization header
	if config.SessionCookie != "" {
		req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
	}

	if config.CSRFToken != "" {
		// Use CSRF token in X-CSRF-Token header
		req.Header.Set("X-CSRF-Token", config.CSRFToken)
	}
	if config.SessionCookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  "TOKEN",
			Value: config.SessionCookie,
		})
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// deleteUbiquityStaticRoute deletes a static route from the router
func deleteUbiquityStaticRoute(config UbiquityConfig, routeID string) error {
	client := createHTTPClient(config)

	// Use the correct endpoint path from the working example
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/%s", config.APIBaseURL, routeID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	// Add essential headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "thread-route-updater/1.0")

	// Add CSRF token header
	if config.CSRFToken != "" {
		req.Header.Set("X-CSRF-Token", config.CSRFToken)
	}

	// Add TOKEN cookie (JWT token)
	if config.SessionCookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  "TOKEN",
			Value: config.SessionCookie,
		})
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// createHTTPClient creates an HTTP client with appropriate settings
func createHTTPClient(config UbiquityConfig) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSSL,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// convertToUbiquityRoutes converts our Route format to Ubiquity format
func convertToUbiquityRoutes(routes []Route) []UbiquityStaticRoute {
	var ubiquityRoutes []UbiquityStaticRoute

	for _, route := range routes {
		// Remove escaping from router name for cleaner display
		cleanRouterName := strings.ReplaceAll(route.RouterName, "\\", "")

		// Use the correct Ubiquiti field structure
		ubiquityRoute := UbiquityStaticRoute{
			Enabled:            true,
			Name:               fmt.Sprintf("Thread route via %s", cleanRouterName),
			Type:               "static-route",
			StaticRouteNexthop: route.ThreadRouterIPv6,
			StaticRouteNetwork: route.CIDR,
			StaticRouteType:    "nexthop-route",
			GatewayType:        "default",
			GatewayDevice:      "1c:0b:8b:12:64:88", // TODO: Get actual gateway device MAC
		}
		ubiquityRoutes = append(ubiquityRoutes, ubiquityRoute)
	}

	return ubiquityRoutes
}

// compareRoutesWithGracePeriod compares current and desired routes with grace period consideration
func compareRoutesWithGracePeriod(current, desired []UbiquityStaticRoute, routeLastSeen map[string]time.Time, gracePeriod time.Duration) ([]UbiquityStaticRoute, []UbiquityStaticRoute) {
	var toAdd []UbiquityStaticRoute
	var toRemove []UbiquityStaticRoute
	currentTime := time.Now()

	// Create a map of desired routes for quick lookup
	desiredMap := make(map[string]UbiquityStaticRoute)
	for _, route := range desired {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		desiredMap[key] = route
	}

	// Find routes to remove (in current but not in desired)
	for _, currentRoute := range current {
		key := fmt.Sprintf("%s->%s", currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop)
		if _, exists := desiredMap[key]; !exists {
			// Only remove Thread routes (routes with our name pattern)
			if strings.Contains(currentRoute.Name, "Thread route via") {
				// Check if we should respect grace period
				if lastSeen, hasLastSeen := routeLastSeen[key]; hasLastSeen {
					timeSinceLastSeen := currentTime.Sub(lastSeen)
					if timeSinceLastSeen < gracePeriod {
						remaining := gracePeriod - timeSinceLastSeen
						remainingStr := formatDuration(remaining)
						fmt.Printf("⏳ Route %s -> %s still within grace period (%s remaining), not removing\n",
							currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop, remainingStr)
						continue
					}
				} else {
					// Route was never seen before - treat as if it was just seen to give it grace period
					gracePeriodStr := formatDuration(gracePeriod)
					fmt.Printf("⏳ Route %s -> %s never seen before, giving grace period (%s), not removing\n",
						currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop, gracePeriodStr)
					// Mark it as seen now so it gets the full grace period
					routeLastSeen[key] = currentTime
					continue
				}
				toRemove = append(toRemove, currentRoute)
			}
		}
	}

	// Find routes to add (in desired but not in current)
	currentMap := make(map[string]bool)
	for _, route := range current {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		currentMap[key] = true
	}

	for _, desiredRoute := range desired {
		key := fmt.Sprintf("%s->%s", desiredRoute.StaticRouteNetwork, desiredRoute.StaticRouteNexthop)
		if !currentMap[key] {
			toAdd = append(toAdd, desiredRoute)
		}
	}

	return toAdd, toRemove
}

// compareRoutes compares current and desired routes to find what needs to be added/removed
// This is the original function kept for backward compatibility
func compareRoutes(current, desired []UbiquityStaticRoute) ([]UbiquityStaticRoute, []UbiquityStaticRoute) {
	var toAdd []UbiquityStaticRoute
	var toRemove []UbiquityStaticRoute

	// Create a map of desired routes for quick lookup
	desiredMap := make(map[string]UbiquityStaticRoute)
	for _, route := range desired {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		desiredMap[key] = route
	}

	// Find routes to remove (in current but not in desired)
	for _, currentRoute := range current {
		key := fmt.Sprintf("%s->%s", currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop)
		if _, exists := desiredMap[key]; !exists {
			// Only remove Thread routes (routes with our name pattern)
			if strings.Contains(currentRoute.Name, "Thread route via") {
				toRemove = append(toRemove, currentRoute)
			}
		}
	}

	// Find routes to add (in desired but not in current)
	currentMap := make(map[string]bool)
	for _, route := range current {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		currentMap[key] = true
	}

	for _, desiredRoute := range desired {
		key := fmt.Sprintf("%s->%s", desiredRoute.StaticRouteNetwork, desiredRoute.StaticRouteNexthop)
		if !currentMap[key] {
			toAdd = append(toAdd, desiredRoute)
		}
	}

	return toAdd, toRemove
}

// loginToUbiquity authenticates with the Ubiquity router and gets a session token
func loginToUbiquity(config *UbiquityConfig) error {
	client := createHTTPClient(*config)

	// Login endpoint
	url := fmt.Sprintf("%s/api/auth/login", config.APIBaseURL)

	loginReq := UbiquityLoginRequest{
		Username: config.Username,
		Password: config.Password,
	}

	jsonData, err := json.Marshal(loginReq)
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
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	// Read the response body first to debug
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Try to parse as the expected format first
	var loginResp UbiquityLoginResponse
	if err := json.Unmarshal(body, &loginResp); err == nil && loginResp.Meta.RC == "ok" {
		// Standard format with meta.rc
		if len(loginResp.Data) > 0 {
			config.SessionToken = loginResp.Data[0].XCsrfToken
		}
	} else {
		// Alternative format - direct user profile response
		var userProfile map[string]interface{}
		if err := json.Unmarshal(body, &userProfile); err != nil {
			return fmt.Errorf("failed to parse login response: %v, body: %s", err, string(body))
		}

		// Check if we have a valid user profile
		if username, ok := userProfile["username"].(string); ok && username == config.Username {
			// Login successful
			// Use the device token as the session token
			if deviceToken, ok := userProfile["deviceToken"].(string); ok {
				config.SessionToken = deviceToken
				config.LastLoginTime = time.Now().Unix()
			}
		} else {
			return fmt.Errorf("login failed: invalid user profile, body: %s", string(body))
		}
	}

	// Extract CSRF token from response headers (but don't override device token)
	csrfToken := resp.Header.Get("X-CSRF-Token")
	if csrfToken != "" {
		config.CSRFToken = csrfToken
	}

	// Also set the session cookie
	for _, cookie := range resp.Cookies() {
		// Ubiquity uses TOKEN cookie instead of unifises
		if cookie.Name == "TOKEN" || cookie.Name == "unifises" {
			// Store the session cookie value for future requests
			config.SessionCookie = cookie.Value
		}
	}

	return nil
}
