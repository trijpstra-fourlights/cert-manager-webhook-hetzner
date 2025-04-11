package internal

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

type Config struct {
	ApiKey, ZoneName, ApiUrl string
}

type RecordResponse struct {
	Records []Record `json:"records"`
	Meta    Meta     `json:"meta"`
}

type ZoneResponse struct {
	Zones []Zone `json:"zones"`
	Meta  Meta   `json:"meta"`
}

type Meta struct {
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Page         int `json:"page"`
	PerPage      int `json:"per_page"`
	LastPage     int `json:"last_page"`
	TotalEntries int `json:"total_entries"`
}

type Record struct {
	Type     string `json:"type"`
	Id       string `json:"id"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
	ZoneId   string `json:"zone_id"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Ttl      int    `json:"ttl"`
}

type Zone struct {
	Id              string       `json:"id"`
	Created         string       `json:"created"`
	Modified        string       `json:"modified"`
	LegacyDnsHost   string       `json:"legacy_dns_host"`
	LegacyNs        []string     `json:"legacy_ns"`
	Name            string       `json:"name"`
	Ns              []string     `json:"ns"`
	Owner           string       `json:"owner"`
	Paused          bool         `json:"paused"`
	Permission      string       `json:"permission"`
	Project         string       `json:"project"`
	Registrar       string       `json:"registrar"`
	Status          string       `json:"status"`
	Ttl             int          `json:"ttl"`
	Verified        string       `json:"verified"`
	RecordsCount    int          `json:"records_count"`
	IsSecondaryDns  bool         `json:"is_secondary_dns"`
	TxtVerification Verification `json:"txt_verification"`
}

type Verification struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

// zoneIdSearcher defines the function signature for searching a zone ID by name.
// This allows mocking the search functionality for testing.
type zoneIdSearcher func(zoneName string) (string, error)

func searchZoneName(searchZone string, searcher zoneIdSearcher) (string, error) {
	// Normalize searchZone by removing any potential trailing dot
	normalizedSearchZone := strings.TrimSuffix(searchZone, ".")
	parts := strings.Split(normalizedSearchZone, ".")

	// Need at least 2 parts (e.g., domain.com) to form a potential zone
	if len(parts) < 2 {
		return "", fmt.Errorf("unable to determine potential zones from searchZone: %s", searchZone)
	}

	// Iterate from the most specific potential zone (e.g., sub.domain.com)
	// down to the least specific (e.g., domain.com).
	// The loop goes from i=0 (full domain) up to len(parts)-2 (TLD+1).
	for i := 0; i <= len(parts)-2; i++ {
		potentialZoneName := strings.Join(parts[i:], ".")
		zoneId, err := searcher(potentialZoneName) // Use the provided searcher function

		if err != nil {
			// Log the error from searchZoneId (e.g., API error, unexpected multiple zones)
			// but continue searching parent domains as the error might be specific to this level.
			klog.Warningf("Error searching for zone ID for '%s': %v. Trying parent domain.", potentialZoneName, err)
			// Continue to the next iteration (parent domain)
		}

		if zoneId != "" {
			// Found the zone successfully
			klog.Infof("Found ZoneName: %s (searched using FQDN: %s)", potentialZoneName, searchZone)
			return potentialZoneName, nil
		}

		// If zoneId is empty and err was nil, it means the zone wasn't found at this level.
		// Continue to the parent domain.
		klog.V(4).Infof("Zone '%s' not found via API, trying parent.", potentialZoneName)
	}

	// If the loop completes without finding a zone ID for any potential zone name
	return "", fmt.Errorf("unable to find a registered Hetzner DNS zone for domain: %s or its parents", searchZone)
}
