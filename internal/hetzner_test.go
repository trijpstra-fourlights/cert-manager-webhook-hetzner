package internal

import (
	"fmt"
	"strings"
	"testing"
)

// mockZoneIdSearcher simulates the behavior of searching for a zone ID.
// It returns a predefined zone ID or error based on the input zone name.
func mockZoneIdSearcher(zones map[string]string, errs map[string]error) zoneIdSearcher {
	return func(zoneName string) (string, error) {
		if err, exists := errs[zoneName]; exists {
			return "", err
		}
		if zoneId, exists := zones[zoneName]; exists {
			return zoneId, nil
		}
		// Simulate zone not found (return empty string and nil error)
		return "", nil
	}
}

func TestSearchZoneName(t *testing.T) {
	testCases := []struct {
		name           string
		searchZone     string
		mockZones      map[string]string // zoneName -> zoneId
		mockErrs       map[string]error  // zoneName -> error
		expectedZone   string
		expectError    bool
		expectedErrMsg string // Expected error message substring
	}{
		{
			name:         "Direct zone match",
			searchZone:   "example.com",
			mockZones:    map[string]string{"example.com": "zone123"},
			mockErrs:     map[string]error{},
			expectedZone: "example.com",
			expectError:  false,
		},
		{
			name:         "Direct zone match with trailing dot",
			searchZone:   "example.com.",
			mockZones:    map[string]string{"example.com": "zone123"},
			mockErrs:     map[string]error{},
			expectedZone: "example.com",
			expectError:  false,
		},
		{
			name:         "Subdomain finds parent zone",
			searchZone:   "sub.example.com",
			mockZones:    map[string]string{"example.com": "zone123"}, // sub.example.com not found, example.com is
			mockErrs:     map[string]error{},
			expectedZone: "example.com",
			expectError:  false,
		},
		{
			name:         "Sub-subdomain finds grandparent zone",
			searchZone:   "deep.sub.example.com",
			mockZones:    map[string]string{"example.com": "zone123"}, // deep.sub.example.com and sub.example.com not found
			mockErrs:     map[string]error{},
			expectedZone: "example.com",
			expectError:  false,
		},
		{
			name:           "Zone not found",
			searchZone:     "unknown.com",
			mockZones:      map[string]string{},
			mockErrs:       map[string]error{},
			expectedZone:   "",
			expectError:    true,
			expectedErrMsg: "unable to find a registered Hetzner DNS zone",
		},
		{
			name:         "Error during search, but parent found",
			searchZone:   "api-error.example.com",
			mockZones:    map[string]string{"example.com": "zone456"},
			mockErrs:     map[string]error{"api-error.example.com": fmt.Errorf("simulated API error")},
			expectedZone: "example.com",
			expectError:  false, // Expect no error overall, as parent is found
		},
		{
			name:       "Persistent error during search",
			searchZone: "error.fail.com",
			mockZones:  map[string]string{},
			mockErrs: map[string]error{
				"error.fail.com": fmt.Errorf("simulated API error 1"),
				"fail.com":       fmt.Errorf("simulated API error 2"),
			},
			expectedZone:   "",
			expectError:    true,
			expectedErrMsg: "unable to find a registered Hetzner DNS zone", // Error because no zone was ever found
		},
		{
			name:           "Invalid input - less than 2 parts",
			searchZone:     "com",
			mockZones:      map[string]string{},
			mockErrs:       map[string]error{},
			expectedZone:   "",
			expectError:    true,
			expectedErrMsg: "unable to determine potential zones",
		},
		{
			name:           "Invalid input - empty string",
			searchZone:     "",
			mockZones:      map[string]string{},
			mockErrs:       map[string]error{},
			expectedZone:   "",
			expectError:    true,
			expectedErrMsg: "unable to determine potential zones",
		},
		{
			name:       "Specific subdomain exists, parent also exists",
			searchZone: "specific.example.com",
			mockZones: map[string]string{
				"specific.example.com": "zone-specific",
				"example.com":          "zone-parent",
			},
			mockErrs:     map[string]error{},
			expectedZone: "specific.example.com", // Should find the most specific one first
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			searcher := mockZoneIdSearcher(tc.mockZones, tc.mockErrs)
			foundZone, err := searchZoneName(tc.searchZone, searcher)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				} else if tc.expectedErrMsg != "" && !strings.Contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("Expected error message containing '%s', but got: %v", tc.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
				if foundZone != tc.expectedZone {
					t.Errorf("Expected zone '%s', but got '%s'", tc.expectedZone, foundZone)
				}
			}
		})
	}
}
