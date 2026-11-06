package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/security"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
)

// handleSecurityScan runs a non-invasive security and CVE review directly
// against a site's configured production healthcheck URL.
func (s *APIServer) handleSecurityScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SiteID string `json:"site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.SiteID) == "" {
		apiErr(w, http.StatusBadRequest, "site_id is required")
		return
	}

	site, err := s.Database.GetSite(req.SiteID)
	if err != nil {
		apiErr(w, http.StatusNotFound, "site not found")
		return
	}

	target := strings.TrimSpace(site.HealthcheckURL)
	if target == "" {
		apiErr(w, http.StatusBadRequest, "healthcheck_url is required for a production security scan")
		return
	}
	parsedTarget, err := url.Parse(target)
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" {
		apiErr(w, http.StatusBadRequest, "healthcheck_url is invalid")
		return
	}

	sshOpts := ssh.NewSSHOptions(site.WPSSHHost, site.WPSSHUser, site.WPSSHPort)
	sshOpts.Key = site.WPSSHKey
	sshClient := s.newSSHClient(sshOpts)
	defer sshClient.Close()

	wpRoot := strings.TrimSpace(site.WPRoot)
	if wpRoot == "" {
		if detected, detectErr := sshClient.DetectWPRoot(); detectErr == nil {
			wpRoot = detected
		}
	}

	components, composerContents, warnings := collectLiveSecurityInventory(sshClient, wpRoot)

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	report, err := security.Scan(ctx, target, security.Options{Timeout: 10 * time.Second})
	if err != nil {
		apiErr(w, http.StatusBadGateway, "security scan failed: "+err.Error())
		return
	}
	for _, warning := range warnings {
		report.AddFindings(security.Finding{
			ID:             "inventory-warning",
			Severity:       security.SeverityInfo,
			Title:          "WordPress inventory is incomplete",
			Description:    warning,
			Recommendation: "Verify SSH and WP-CLI access for this site, then run the scan again.",
		})
	}

	maxRequests := 0
	if raw := strings.TrimSpace(os.Getenv("WPSCAN_MAX_REQUESTS")); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
			maxRequests = parsed
		}
	}
	cveFindings := security.LookupVulnerabilities(
		ctx,
		components,
		composerContents,
		security.VulnerabilityOptions{
			WPScanToken: os.Getenv("WPSCAN_API_TOKEN"),
			MaxRequests: maxRequests,
			Cache:       &dbCVECache{db: s.Database},
		},
	)
	report.AddFindings(cveFindings...)
	jsonResp(w, http.StatusOK, report)
}

func collectLiveSecurityInventory(client ssh.Client, wpRoot string) ([]security.Component, []string, []string) {
	components := make([]security.Component, 0)
	warnings := make([]string, 0)
	if client == nil || strings.TrimSpace(wpRoot) == "" {
		warnings = append(warnings, "WordPress root is not configured or could not be detected.")
		return components, nil, warnings
	}

	coreCommand := fmt.Sprintf("cd %s && wp core version --skip-plugins --skip-themes 2>/dev/null", ssh.ShellQuote(wpRoot))
	if coreVersion, err := client.RunCommand(coreCommand); err == nil && strings.TrimSpace(coreVersion) != "" {
		components = append(components, security.Component{
			Type:    "core",
			Slug:    "wordpress",
			Name:    "WordPress",
			Version: strings.TrimSpace(coreVersion),
			Active:  true,
		})
	} else {
		warnings = append(warnings, "WordPress core version could not be read over WP-CLI: "+shortScanError(err))
	}

	if phpVersion, err := client.GetPHPVersion(wpRoot); err == nil && strings.TrimSpace(phpVersion) != "" {
		components = append(components, security.Component{
			Type:    "php",
			Slug:    "php",
			Name:    "PHP",
			Version: strings.TrimSpace(phpVersion),
			Active:  true,
		})
	}

	pluginsCommand := fmt.Sprintf("cd %s && wp plugin list --skip-plugins --skip-themes --format=json --fields=name,status,version 2>/dev/null", ssh.ShellQuote(wpRoot))
	if pluginsOut, err := client.RunCommand(pluginsCommand); err == nil {
		var plugins []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		if data := extractJSONArray(pluginsOut); data != "" {
			if err := json.Unmarshal([]byte(data), &plugins); err == nil {
				for _, plugin := range plugins {
					status := strings.ToLower(strings.TrimSpace(plugin.Status))
					if status != "active" && status != "must-use" {
						continue
					}
					if plugin.Name == "" || plugin.Version == "" {
						continue
					}
					components = append(components, security.Component{
						Type:    "plugin",
						Slug:    plugin.Name,
						Name:    plugin.Name,
						Version: plugin.Version,
						Active:  true,
					})
				}
			}
		}
	} else {
		warnings = append(warnings, "Plugin inventory could not be read over WP-CLI: "+shortScanError(err))
	}

	themesCommand := fmt.Sprintf("cd %s && wp theme list --skip-plugins --skip-themes --format=json --fields=name,status,version 2>/dev/null", ssh.ShellQuote(wpRoot))
	if themesOut, err := client.RunCommand(themesCommand); err == nil {
		var themes []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		if data := extractJSONArray(themesOut); data != "" {
			if err := json.Unmarshal([]byte(data), &themes); err == nil {
				for _, theme := range themes {
					status := strings.ToLower(strings.TrimSpace(theme.Status))
					if status != "active" && status != "parent" {
						continue
					}
					if theme.Name == "" || theme.Version == "" {
						continue
					}
					components = append(components, security.Component{
						Type:    "theme",
						Slug:    theme.Name,
						Name:    theme.Name,
						Version: theme.Version,
						Active:  true,
					})
				}
			}
		}
	} else {
		warnings = append(warnings, "Theme inventory could not be read over WP-CLI: "+shortScanError(err))
	}

	return components, collectRemoteComposerJSON(client, wpRoot), warnings
}

func shortScanError(err error) string {
	if err == nil {
		return "empty response"
	}
	message := strings.TrimSpace(err.Error())
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		message = message[:idx]
	}
	if len(message) > 300 {
		message = message[:300] + "..."
	}
	return message
}

func collectRemoteComposerJSON(client ssh.Client, wpRoot string) []string {
	if client == nil || strings.TrimSpace(wpRoot) == "" {
		return nil
	}
	findCommand := fmt.Sprintf(
		"find %s/wp-content -type f -path '*/vendor/composer/installed.json' -print 2>/dev/null",
		ssh.ShellQuote(strings.TrimRight(wpRoot, "/")),
	)
	output, err := client.RunCommand(findCommand)
	if err != nil {
		return nil
	}
	contents := make([]string, 0)
	for _, path := range strings.Split(strings.TrimSpace(output), "\n") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		data, err := client.RunCommand("cat " + ssh.ShellQuote(path))
		if err == nil && strings.TrimSpace(data) != "" {
			contents = append(contents, data)
		}
		if len(contents) >= 20 {
			break
		}
	}
	return contents
}
