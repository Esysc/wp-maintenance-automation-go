package security

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Component struct {
	Type    string `json:"type"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Active  bool   `json:"active"`
}

type VulnerabilityOptions struct {
	WPScanToken      string
	MaxRequests      int
	Cache            Cache
	WPScanBaseURL    string
	OSVBaseURL       string
	EndOfLifeBaseURL string
	HTTPClient       *http.Client
}

const (
	defaultWPScanBaseURL = "https://wpscan.com/api/v3"
	defaultOSVBaseURL    = "https://api.osv.dev"
	defaultEOLBaseURL    = "https://endoflife.date/api"
)

var errRateLimited = errors.New("rate limited")

// LookupVulnerabilities enriches a security review with known CVEs from
// WPScan (WordPress core, plugins, themes), OSV (Composer dependencies), and
// the PHP end-of-life feed. Provider failures are returned as informational
// findings so the rehearsal can continue.
func LookupVulnerabilities(ctx context.Context, components []Component, composerContents []string, opts VulnerabilityOptions) []Finding {
	if opts.Cache == nil {
		opts.Cache = NewMemoryCache(24 * time.Hour)
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}

	findings := make([]Finding, 0)
	wpFindings, err := lookupWPScan(ctx, components, opts)
	if err != nil {
		findings = append(findings, providerFailure("wpscan", err))
	} else {
		findings = append(findings, wpFindings...)
	}

	if php := componentByType(components, "php"); php.Version != "" {
		phpFindings, err := lookupPHPEOL(ctx, php, opts)
		if err != nil {
			findings = append(findings, providerFailure("php-eol", err))
		} else {
			findings = append(findings, phpFindings...)
		}
	}

	packages := ComposerPackages(composerContents)
	osvFindings, err := lookupOSV(ctx, packages, opts)
	if err != nil {
		findings = append(findings, providerFailure("osv", err))
	} else {
		findings = append(findings, osvFindings...)
	}

	return findings
}

func providerFailure(provider string, err error) Finding {
	return Finding{
		ID:             "cve-lookup-" + provider + "-failed",
		Severity:       SeverityInfo,
		Title:          "CVE lookup provider unavailable",
		Description:    fmt.Sprintf("The %s vulnerability lookup could not complete.", provider),
		Recommendation: "Check outbound network access and the provider configuration, then run the rehearsal again.",
		Evidence:       err.Error(),
	}
}

func componentByType(components []Component, componentType string) Component {
	for _, component := range components {
		if component.Type == componentType {
			return component
		}
	}
	return Component{}
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte) error
}

type memoryCacheEntry struct {
	data    []byte
	expires time.Time
}

type MemoryCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]memoryCacheEntry
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &MemoryCache{ttl: ttl, entries: map[string]memoryCacheEntry{}}
}

func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.data, true
}

func (c *MemoryCache) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = memoryCacheEntry{data: value, expires: time.Now().Add(c.ttl)}
	return nil
}

// ---------------------------------------------------------------------------
// WPScan
// ---------------------------------------------------------------------------

type wpscanVulnerability struct {
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	FixedIn      string              `json:"fixed_in"`
	IntroducedIn string              `json:"introduced_in"`
	Severity     string              `json:"severity"`
	CVSS         wpscanCVSS          `json:"cvss"`
	References   map[string][]string `json:"references"`
}

type wpscanCVSS struct {
	Score  float64 `json:"score"`
	Vector string  `json:"vector"`
}

type wpscanEntry struct {
	Vulnerabilities []wpscanVulnerability `json:"vulnerabilities"`
}

func lookupWPScan(ctx context.Context, components []Component, opts VulnerabilityOptions) ([]Finding, error) {
	if strings.TrimSpace(opts.WPScanToken) == "" {
		return []Finding{{
			ID:             "cve-lookup-not-configured",
			Severity:       SeverityInfo,
			Title:          "WordPress CVE lookup is not configured",
			Description:    "WPScan API lookups are skipped because WPSCAN_API_TOKEN is not set.",
			Recommendation: "Create a WPScan API token, set WPSCAN_API_TOKEN for the API container, and run the rehearsal again.",
		}}, nil
	}

	base := strings.TrimRight(opts.WPScanBaseURL, "/")
	if base == "" {
		base = defaultWPScanBaseURL
	}
	budget := opts.MaxRequests
	if budget <= 0 {
		budget = 20
	}

	findings := make([]Finding, 0)
	skipped := make([]string, 0)
	for _, component := range components {
		if !component.Active || component.Slug == "" || component.Version == "" {
			continue
		}
		switch component.Type {
		case "core", "plugin", "theme":
		default:
			continue
		}

		key := fmt.Sprintf("wpscan:%s:%s:%s", component.Type, component.Slug, component.Version)
		if cached, ok := opts.Cache.Get(key); ok {
			var cachedFindings []Finding
			if json.Unmarshal(cached, &cachedFindings) == nil {
				findings = append(findings, cachedFindings...)
			}
			continue
		}

		if budget <= 0 {
			skipped = append(skipped, component.Name)
			continue
		}
		budget--

		vulnerabilities, err := fetchWPScan(ctx, opts.HTTPClient, base, opts.WPScanToken, component)
		if err != nil {
			if errors.Is(err, errRateLimited) {
				skipped = append(skipped, component.Name)
				budget = 0
				continue
			}
			return findings, err
		}

		componentFindings := wpscanFindings(component, vulnerabilities)
		encoded, _ := json.Marshal(componentFindings)
		_ = opts.Cache.Set(key, encoded)
		findings = append(findings, componentFindings...)
	}

	if len(skipped) > 0 {
		findings = append(findings, Finding{
			ID:             "cve-lookup-incomplete",
			Severity:       SeverityInfo,
			Title:          "CVE lookup was incomplete",
			Description:    fmt.Sprintf("%d component(s) were not checked because the lookup budget or API rate limit was reached.", len(skipped)),
			Recommendation: "Increase WPSCAN_MAX_REQUESTS or use a WPScan plan with a higher rate limit.",
			Evidence:       strings.Join(skipped, ", "),
		})
	}

	return findings, nil
}

func fetchWPScan(ctx context.Context, client *http.Client, base, token string, component Component) ([]wpscanVulnerability, error) {
	var path string
	switch component.Type {
	case "core":
		path = "/wordpresses/" + url.PathEscape(component.Version)
	case "plugin":
		path = "/plugins/" + url.PathEscape(component.Slug)
	case "theme":
		path = "/themes/" + url.PathEscape(component.Slug)
	default:
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("User-Agent", "WP-Maintenance-CVE-Review/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wpscan returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var entries map[string]wpscanEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("invalid wpscan response: %w", err)
	}
	for _, entry := range entries {
		return entry.Vulnerabilities, nil
	}
	return nil, nil
}

func wpscanFindings(component Component, vulnerabilities []wpscanVulnerability) []Finding {
	findings := make([]Finding, 0)
	for _, vulnerability := range vulnerabilities {
		fixed := strings.TrimSpace(vulnerability.FixedIn)
		introduced := strings.TrimSpace(vulnerability.IntroducedIn)
		if !versionAffected(component.Version, introduced, fixed) {
			continue
		}

		severity := severityFromLabel(vulnerability.Severity)
		if severity == "" {
			severity = severityFromCVSS(vulnerability.CVSS.Score)
		}

		cves := dedupeStrings(vulnerability.References["cve"])
		urls := dedupeStrings(vulnerability.References["url"])
		reference := ""
		if len(urls) > 0 {
			reference = urls[0]
		} else if len(cves) > 0 {
			reference = "https://www.cve.org/CVERecord?id=" + cves[0]
		}

		id := strings.TrimSpace(vulnerability.ID)
		if id == "" && len(cves) > 0 {
			id = cves[0]
		}
		if id == "" {
			id = component.Slug + "-" + fixed
		}

		recommendation := fmt.Sprintf("Remove or replace %s; no fixed version is published.", component.Name)
		if fixed != "" {
			recommendation = fmt.Sprintf("Update %s to %s or later.", component.Name, fixed)
		}

		evidenceParts := []string{"installed " + component.Version}
		if fixed != "" {
			evidenceParts = append(evidenceParts, "fixed in "+fixed)
		}
		if len(cves) > 0 {
			evidenceParts = append(evidenceParts, "CVEs: "+strings.Join(cves, ", "))
		}
		if vulnerability.CVSS.Score > 0 {
			evidenceParts = append(evidenceParts, fmt.Sprintf("CVSS %.1f", vulnerability.CVSS.Score))
			if vulnerability.CVSS.Vector != "" {
				evidenceParts = append(evidenceParts, vulnerability.CVSS.Vector)
			}
		}

		action := "Run the update in staging first, then production:\n"
		switch component.Type {
		case "core":
			action += "  wp core update"
		case "plugin":
			action += "  wp plugin update " + component.Slug
		case "theme":
			action += "  wp theme update " + component.Slug
		}
		if fixed != "" {
			action += "\nTarget: " + fixed + " or later."
		} else {
			action += "\nNo fixed version is published; remove or replace the component."
		}

		findings = append(findings, Finding{
			ID:             "wpscan-" + id,
			Severity:       severity,
			Title:          component.Name + ": " + strings.TrimSpace(vulnerability.Title),
			Description:    fmt.Sprintf("A known vulnerability affects %s %s.", component.Name, component.Version),
			Recommendation: recommendation,
			Action:         action,
			Evidence:       strings.Join(evidenceParts, "; "),
			Reference:      reference,
		})
	}
	return findings
}

func versionAffected(installed, introduced, fixed string) bool {
	if fixed != "" && hasVersionNumber(fixed) && compareVersions(installed, fixed) >= 0 {
		return false
	}
	if introduced != "" && hasVersionNumber(introduced) && compareVersions(installed, introduced) < 0 {
		return false
	}
	return true
}

var versionNumberRe = regexp.MustCompile(`\d+`)

func hasVersionNumber(value string) bool {
	return versionNumberRe.MatchString(value)
}

func startsWithDigit(value string) bool {
	if value == "" {
		return false
	}
	return value[0] >= '0' && value[0] <= '9'
}

func compareVersions(a, b string) int {
	an := versionNumberRe.FindAllString(a, -1)
	bn := versionNumberRe.FindAllString(b, -1)
	maxLen := len(an)
	if len(bn) > maxLen {
		maxLen = len(bn)
	}
	for i := 0; i < maxLen; i++ {
		ai, bi := 0, 0
		if i < len(an) {
			ai, _ = strconv.Atoi(an[i])
		}
		if i < len(bn) {
			bi, _ = strconv.Atoi(bn[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func selectFixedVersion(installed string, candidates []string) string {
	installedMajor := majorVersion(installed)
	sameMajor := make([]string, 0)
	newer := make([]string, 0)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !startsWithDigit(candidate) {
			continue
		}
		if compareVersions(candidate, installed) <= 0 {
			continue
		}
		newer = append(newer, candidate)
		if majorVersion(candidate) == installedMajor {
			sameMajor = append(sameMajor, candidate)
		}
	}
	pool := sameMajor
	if len(pool) == 0 {
		pool = newer
	}
	if len(pool) == 0 {
		return ""
	}
	best := pool[0]
	for _, candidate := range pool[1:] {
		if compareVersions(candidate, best) < 0 {
			best = candidate
		}
	}
	return best
}

func majorVersion(value string) string {
	numbers := versionNumberRe.FindAllString(value, -1)
	if len(numbers) == 0 {
		return ""
	}
	return numbers[0]
}

func severityFromCVSS(score float64) Severity {
	switch {
	case score >= 9:
		return SeverityCritical
	case score >= 7:
		return SeverityHigh
	case score >= 4:
		return SeverityMedium
	case score > 0:
		return SeverityLow
	default:
		return SeverityMedium
	}
}

func severityFromLabel(value string) Severity {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium", "moderate":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "info", "informational", "none":
		return SeverityInfo
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// ---------------------------------------------------------------------------
// Composer / OSV
// ---------------------------------------------------------------------------

type composerPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

func ComposerPackages(contents []string) []Component {
	seen := map[string]bool{}
	packages := make([]Component, 0)
	for _, data := range contents {
		for _, pkg := range parseComposerInstalled([]byte(data)) {
			if pkg.Type == "metapackage" || pkg.Name == "" || pkg.Version == "" {
				continue
			}
			version := strings.TrimPrefix(strings.TrimSpace(pkg.Version), "v")
			if !startsWithDigit(version) {
				continue
			}
			key := pkg.Name + "@" + version
			if seen[key] {
				continue
			}
			seen[key] = true
			packages = append(packages, Component{
				Type:    "composer",
				Slug:    pkg.Name,
				Name:    pkg.Name,
				Version: version,
				Active:  true,
			})
		}
	}
	return packages
}

func findComposerPackages(restoreDir string) []Component {
	if strings.TrimSpace(restoreDir) == "" {
		return nil
	}
	contents := make([]string, 0)
	_ = filepath.WalkDir(restoreDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "installed.json" {
			return nil
		}
		if !strings.Contains(filepath.ToSlash(path), "/vendor/composer/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		contents = append(contents, string(data))
		return nil
	})
	return ComposerPackages(contents)
}

func parseComposerInstalled(data []byte) []composerPackage {
	var wrapped struct {
		Packages []composerPackage `json:"packages"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Packages) > 0 {
		return wrapped.Packages
	}
	var direct []composerPackage
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct
	}
	return nil
}

type osvVulnerability struct {
	ID       string   `json:"id"`
	Summary  string   `json:"summary"`
	Details  string   `json:"details"`
	Aliases  []string `json:"aliases"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	Affected []struct {
		Ranges []struct {
			Events []struct {
				Fixed string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
	References []struct {
		URL string `json:"url"`
	} `json:"references"`
}

type osvQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

func lookupOSV(ctx context.Context, packages []Component, opts VulnerabilityOptions) ([]Finding, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	base := strings.TrimRight(opts.OSVBaseURL, "/")
	if base == "" {
		base = defaultOSVBaseURL
	}

	findings := make([]Finding, 0)
	uncached := make([]Component, 0)
	for _, pkg := range packages {
		key := "osv:Packagist:" + pkg.Name + ":" + pkg.Version
		if cached, ok := opts.Cache.Get(key); ok {
			var stored []Finding
			if json.Unmarshal(cached, &stored) == nil {
				findings = append(findings, stored...)
			}
			continue
		}
		uncached = append(uncached, pkg)
	}

	if len(uncached) == 0 {
		return findings, nil
	}

	for _, pkg := range uncached {
		vulnerabilities, err := queryOSV(ctx, opts.HTTPClient, base, pkg)
		if err != nil {
			return findings, err
		}
		vulnFindings := osvFindings(pkg, vulnerabilities)
		encoded, _ := json.Marshal(vulnFindings)
		_ = opts.Cache.Set("osv:Packagist:"+pkg.Name+":"+pkg.Version, encoded)
		findings = append(findings, vulnFindings...)
	}
	return findings, nil
}

func queryOSV(ctx context.Context, client *http.Client, base string, pkg Component) ([]osvVulnerability, error) {
	var query osvQuery
	query.Package.Name = pkg.Name
	query.Package.Ecosystem = "Packagist"
	query.Version = pkg.Version

	body, _ := json.Marshal(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/query", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WP-Maintenance-CVE-Review/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSV returned HTTP %d", resp.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var response struct {
		Vulns []osvVulnerability `json:"vulns"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("invalid OSV response: %w", err)
	}
	return response.Vulns, nil
}

func osvFindings(pkg Component, vulnerabilities []osvVulnerability) []Finding {
	const maxFindingsPerPackage = 10
	findings := make([]Finding, 0)
	for _, vulnerability := range vulnerabilities {
		if len(findings) >= maxFindingsPerPackage {
			findings = append(findings, Finding{
				ID:             "osv-advisories-truncated",
				Severity:       SeverityInfo,
				Title:          "Additional dependency advisories omitted",
				Description:    fmt.Sprintf("More than %d advisories affect %s %s. Only the first %d are shown.", maxFindingsPerPackage, pkg.Name, pkg.Version, maxFindingsPerPackage),
				Recommendation: "Review the OSV advisory list for the full set and update the dependency.",
				Reference:      "https://osv.dev/list?q=" + url.QueryEscape(pkg.Name),
			})
			break
		}
		severity := severityFromLabel(vulnerability.DatabaseSpecific.Severity)
		if severity == "" {
			for _, score := range vulnerability.Severity {
				if parsed, err := strconv.ParseFloat(score.Score, 64); err == nil {
					severity = severityFromCVSS(parsed)
					break
				}
			}
		}
		if severity == "" {
			severity = SeverityMedium
		}

		fixed := ""
		fixedCandidates := make([]string, 0)
		for _, affected := range vulnerability.Affected {
			for _, rng := range affected.Ranges {
				for _, event := range rng.Events {
					if event.Fixed != "" {
						fixedCandidates = append(fixedCandidates, event.Fixed)
					}
				}
			}
		}
		fixed = selectFixedVersion(pkg.Version, fixedCandidates)

		description := strings.TrimSpace(vulnerability.Summary)
		if description == "" {
			description = firstLine(vulnerability.Details, 300)
		}
		recommendation := fmt.Sprintf("Update or remove %s; no fixed version is listed in the advisory.", pkg.Name)
		if fixed != "" {
			recommendation = fmt.Sprintf("Update %s to %s or later.", pkg.Name, fixed)
		}

		evidence := []string{"installed " + pkg.Version}
		if fixed != "" {
			evidence = append(evidence, "fixed in "+fixed)
		}
		if len(vulnerability.Aliases) > 0 {
			evidence = append(evidence, "aliases: "+strings.Join(vulnerability.Aliases, ", "))
		}

		reference := ""
		if len(vulnerability.References) > 0 {
			reference = vulnerability.References[0].URL
		}

		action := fmt.Sprintf("Update the dependency:\n  composer update %s", pkg.Name)
		if fixed != "" {
			action += "\nTarget: " + fixed + " or later."
		}
		action += "\nIf it is bundled inside a plugin or theme, update that parent component and redeploy."

		findings = append(findings, Finding{
			ID:             "osv-" + vulnerability.ID,
			Severity:       severity,
			Title:          "Vulnerable Composer dependency: " + pkg.Name,
			Description:    description,
			Recommendation: recommendation,
			Action:         action,
			Evidence:       strings.Join(evidence, "; "),
			Reference:      reference,
		})
	}
	return findings
}

func firstLine(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		value = value[:idx]
	}
	if len(value) > limit {
		value = value[:limit] + "..."
	}
	return value
}

// ---------------------------------------------------------------------------
// PHP end-of-life
// ---------------------------------------------------------------------------

type phpCycle struct {
	Cycle string      `json:"cycle"`
	EOL   interface{} `json:"eol"`
}

func lookupPHPEOL(ctx context.Context, php Component, opts VulnerabilityOptions) ([]Finding, error) {
	cycle := phpCycleFromVersion(php.Version)
	if cycle == "" {
		return nil, nil
	}
	key := "eol:php:" + cycle
	if cached, ok := opts.Cache.Get(key); ok {
		var stored []Finding
		if json.Unmarshal(cached, &stored) == nil {
			return stored, nil
		}
	}

	base := strings.TrimRight(opts.EndOfLifeBaseURL, "/")
	if base == "" {
		base = defaultEOLBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/php.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WP-Maintenance-CVE-Review/1.0")
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endoflife.date returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var cycles []phpCycle
	if err := json.Unmarshal(body, &cycles); err != nil {
		return nil, fmt.Errorf("invalid endoflife.date response: %w", err)
	}

	var findings []Finding
	for _, candidate := range cycles {
		if candidate.Cycle != cycle {
			continue
		}
		if eol, ok := eolValue(candidate.EOL); ok {
			findings = append(findings, Finding{
				ID:             "php-eol",
				Severity:       SeverityHigh,
				Title:          "PHP version is end-of-life",
				Description:    fmt.Sprintf("PHP %s is no longer supported by the PHP project.", php.Version),
				Recommendation: "Upgrade to a supported PHP release and re-run the rehearsal.",
				Evidence:       "EOL: " + eol,
			})
		}
		break
	}
	encoded, _ := json.Marshal(findings)
	_ = opts.Cache.Set(key, encoded)
	return findings, nil
}

func phpCycleFromVersion(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func eolValue(value interface{}) (string, bool) {
	switch v := value.(type) {
	case bool:
		if v {
			return "yes", true
		}
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		if date, err := time.Parse("2006-01-02", v); err == nil {
			if !date.After(time.Now()) {
				return v, true
			}
			return "", false
		}
		return v, true
	}
	return "", false
}
