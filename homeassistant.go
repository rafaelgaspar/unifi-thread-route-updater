package main

import (
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type haDataset struct {
	Dataset string `json:"dataset"` // hex-encoded Thread operational dataset TLV
}

// pollHomeAssistant periodically fetches Thread datasets from Home Assistant and
// extracts the Mesh Local Prefix (TLV type 0x07) as a Thread mesh prefix source.
func pollHomeAssistant(state *DaemonState, done <-chan struct{}) {
	cfg := state.HomeAssistantConfig
	if cfg.URL == "" || cfg.Token == "" {
		return
	}
	runPoller(done, 5*time.Minute, "home assistant thread datasets", func() error {
		return fetchHAThreadPrefixes(state, cfg)
	})
}

func fetchHAThreadPrefixes(state *DaemonState, cfg HomeAssistantConfig) error {
	url := cfg.URL + "/api/thread/datasets"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSSL},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var datasets []haDataset
	if err := json.NewDecoder(resp.Body).Decode(&datasets); err != nil {
		return err
	}

	for _, ds := range datasets {
		prefix := parseMeshLocalPrefix(ds.Dataset)
		if prefix == "" {
			continue
		}
		state.mu.Lock()
		if _, known := state.ThreadMeshPrefixes[prefix]; !known {
			logInfo("Discovered Thread mesh prefix from Home Assistant: %s", prefix)
		}
		state.ThreadMeshPrefixes[prefix] = time.Now()
		state.mu.Unlock()
	}
	return nil
}

// parseMeshLocalPrefix decodes a hex-encoded Thread operational dataset TLV and
// returns the Mesh Local Prefix (type 0x07) as a /64 CIDR, or "" if not found.
func parseMeshLocalPrefix(hexDataset string) string {
	data, err := hex.DecodeString(hexDataset)
	if err != nil {
		logDebug("parseMeshLocalPrefix: hex decode error: %v", err)
		return ""
	}
	for i := 0; i+1 < len(data); {
		tlvType := data[i]
		tlvLen := int(data[i+1])
		i += 2
		if i+tlvLen > len(data) {
			break
		}
		val := data[i : i+tlvLen]
		i += tlvLen

		// Type 0x07 = Mesh Local Prefix, always 8 bytes (/64)
		if tlvType == 0x07 && tlvLen == 8 {
			prefix := make(net.IP, 16)
			copy(prefix, val)
			if (prefix[0] & 0xfe) != 0xfc {
				continue
			}
			masked := maskPrefix(prefix, 64)
			return fmt.Sprintf("%s/64", masked.String())
		}
	}
	return ""
}
