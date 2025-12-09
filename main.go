package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"encoding/json"
	"fmt"
	"os"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/trijpstra-fourlights/cert-manager-webhook-hetzner/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&hetznerDNSProviderSolver{},
	)
}

type hetznerDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type hetznerDNSProviderConfig struct {
	SecretRef string `json:"secretName"`
	ZoneName  string `json:"zoneName"`
	ApiUrl    string `json:"apiUrl"`
}

func (c *hetznerDNSProviderSolver) Name() string {
	return "hetzner"
}

func (c *hetznerDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.V(6).Infof("call function Present: namespace=%s, zone=%s, fqdn=%s",
		ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	config, err := clientConfig(c, ch)

	if err != nil {
		return fmt.Errorf("unable to get secret `%s`; %v", ch.ResourceNamespace, err)
	}

	addTxtRecord(config, ch)

	klog.Infof("Presented txt record %v", ch.ResolvedFQDN)

	return nil
}

func (c *hetznerDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	config, err := clientConfig(c, ch)

	if err != nil {
		return fmt.Errorf("unable to get secret `%s`; %v", ch.ResourceNamespace, err)
	}

	zoneId, err := searchZoneId(config)

	if err != nil {
		return fmt.Errorf("unable to find id for zone name `%s`; %v", config.ZoneName, err)
	}

	var url = config.ApiUrl + "/records?zone_id=" + zoneId

	// Get all DNS records
	dnsRecords, err := callDnsApi(url, "GET", nil, config)

	if err != nil {
		return fmt.Errorf("unable to get DNS records %v", err)
	}

	// Unmarshall response
	records := internal.RecordResponse{}
	readErr := json.Unmarshal(dnsRecords, &records)

	if readErr != nil {
		return fmt.Errorf("unable to unmarshal response %v", readErr)
	}

	var recordId string
	name := recordName(ch.ResolvedFQDN, config.ZoneName)
	for i := len(records.Records) - 1; i >= 0; i-- {
		if strings.EqualFold(records.Records[i].Name, name) {
			recordId = records.Records[i].Id
			break
		}
	}

	// Delete TXT record
	url = config.ApiUrl + "/records/" + recordId
	del, err := callDnsApi(url, "DELETE", nil, config)

	if err != nil {
		klog.Error(err)
	}
	klog.Infof("Delete TXT record result: %s", string(del))
	return nil
}

func (c *hetznerDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	k8sClient, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = k8sClient

	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (hetznerDNSProviderConfig, error) {
	cfg := hetznerDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func stringFromSecretData(secretData map[string][]byte, key string) (string, error) {
	data, ok := secretData[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

func addTxtRecord(config internal.Config, ch *v1alpha1.ChallengeRequest) {
	url := config.ApiUrl + "/records"

	name := recordName(ch.ResolvedFQDN, config.ZoneName)
	zoneId, err := searchZoneId(config)

	if err != nil {
		klog.Errorf("unable to find id for zone name `%s`; %v", config.ZoneName, err)
	}

	var jsonStr = fmt.Sprintf(`{"value":%q, "ttl":120, "type":"TXT", "name":%q, "zone_id":%q}`, ch.Key, name, zoneId)

	add, err := callDnsApi(url, "POST", bytes.NewBuffer([]byte(jsonStr)), config)

	if err != nil {
		klog.Error(err)
	}
	klog.Infof("Added TXT record result: %s", string(add))
}

func clientConfig(c *hetznerDNSProviderSolver, ch *v1alpha1.ChallengeRequest) (internal.Config, error) {
	var config internal.Config

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return config, err
	}
	config.ZoneName = cfg.ZoneName
	config.ApiUrl = cfg.ApiUrl

	secretName := cfg.SecretRef
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})

	if err != nil {
		return config, fmt.Errorf("unable to get secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	apiKey, err := stringFromSecretData(sec.Data, "api-key")
	config.ApiKey = apiKey

	if err != nil {
		return config, fmt.Errorf("unable to get api-key from secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	// Get ZoneName by api search if not provided by config
	if config.ZoneName == "" {
		// Use ch.ResolvedZone which should be the FQDN minus the challenge part
		searchDomain := ch.ResolvedZone
		// Ensure searchDomain has a trailing dot for consistency, although searchZoneName handles it
		if !strings.HasSuffix(searchDomain, ".") {
			searchDomain += "."
		}
		klog.V(4).Infof("ZoneName not provided, attempting to search using: %s", searchDomain)
		foundZone, err := searchZoneName(config, searchDomain)
		if err != nil {
			return config, fmt.Errorf("error searching for zone for %s: %v", searchDomain, err)
		}
		config.ZoneName = foundZone
		klog.V(2).Infof("Found ZoneName '%s' for domain '%s'", foundZone, searchDomain)
	}

	// Default API URL if not provided
	if config.ApiUrl == "" {
		config.ApiUrl = "https://api.hetzner.cloud/v1"
		klog.V(4).Infof("ApiUrl not provided, using default: %s", config.ApiUrl)
	}

	return config, nil
}

/*
Domain name in Hetzner is divided in 2 parts: record + zone name. API works
with record name that is FQDN without zone name. Subdomains is a part of
record name and is separated by "."
*/
// recordName extracts the relative record name from the FQDN.
// Example: fqdn = _acme-challenge.www.example.com., domain = example.com. -> _acme-challenge.www
// Example: fqdn = _acme-challenge.example.com., domain = example.com. -> _acme-challenge
// Example: fqdn = example.com., domain = example.com. -> @ (Hetzner uses @ for the zone apex record name)
func recordName(fqdn, domain string) string {
	// Normalize by removing trailing dots, which might be present in FQDNs/Zones
	fqdnNormalized := strings.TrimSuffix(fqdn, ".")
	domainNormalized := strings.TrimSuffix(domain, ".")

	// Handle apex domain case (though unlikely for ACME challenges which are prefixed)
	if fqdnNormalized == domainNormalized {
		klog.V(4).Infof("recordName: FQDN '%s' matches domain '%s', returning '@' for apex.", fqdn, domain)
		return "@"
	}

	// Construct the expected suffix (zone name)
	suffix := "." + domainNormalized

	// Check if FQDN ends with the zone suffix and extract the prefix
	if strings.HasSuffix(fqdnNormalized, suffix) {
		record := strings.TrimSuffix(fqdnNormalized, suffix)
		// Ensure record is not empty after trimming (shouldn't happen with _acme-challenge)
		if record == "" {
			klog.Errorf("recordName: FQDN '%s' resulted in empty record name after trimming zone '%s'. This is unexpected.", fqdn, domain)
			return "" // Return empty string on unexpected result
		}
		klog.V(4).Infof("recordName: FQDN '%s', domain '%s', extracted record name '%s'", fqdn, domain, record)
		return record
	}

	// If it doesn't end with the expected suffix, the zone/FQDN combination is likely incorrect.
	klog.Errorf("recordName: FQDN '%s' does not seem to belong to zone '%s'. Returning empty string.", fqdn, domain)
	return ""
}

func callDnsApi(url, method string, body io.Reader, config internal.Config) ([]byte, error) {
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to execute request %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer " + config.ApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Fatal(err)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return respBody, nil
	}

	text := "Error calling API status:" + resp.Status + " url: " + url + " method: " + method
	klog.Error(text)
	return nil, errors.New(text)
}

func searchZoneId(config internal.Config) (string, error) {
	url := config.ApiUrl + "/zones?name=" + config.ZoneName

	// Get Zone configuration
	zoneRecords, err := callDnsApi(url, "GET", nil, config)

	if err != nil {
		// Propagate API call errors
		return "", fmt.Errorf("API error getting zone info for '%s': %v", config.ZoneName, err)
	}

	// Unmarshall response
	zones := internal.ZoneResponse{}
	readErr := json.Unmarshal(zoneRecords, &zones)

	if readErr != nil {
		return "", fmt.Errorf("unable to unmarshal zone response for '%s': %v", config.ZoneName, readErr)
	}

	// Check the number of zones returned
	if zones.Meta.Pagination.TotalEntries == 0 {
		// Explicitly return empty string and nil error if zone is not found
		return "", nil
	}

	if zones.Meta.Pagination.TotalEntries > 1 {
		// This case should ideally not happen if Hetzner API guarantees unique zone names
		klog.Warningf("Found multiple zones (%d) for name '%s', using the first one.", zones.Meta.Pagination.TotalEntries, config.ZoneName)
		// Return error as this is unexpected
		return "", fmt.Errorf("unexpected number of zones (%d) found for name '%s'", zones.Meta.Pagination.TotalEntries, config.ZoneName)
	}

	// Exactly one zone found
	return zones.Zones[0].Id, nil
}

// searchZoneName attempts to find the correct Hetzner zone name for a given FQDN (searchZone)
// by iteratively querying parent domains. searchZone should typically be the value from
// ChallengeRequest.ResolvedZone.
func searchZoneName(config internal.Config, searchZone string) (string, error) {
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
		// Temporarily set ZoneName in config for searchZoneId call
		config.ZoneName = potentialZoneName
		zoneId, err := searchZoneId(config) // Capture error

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
