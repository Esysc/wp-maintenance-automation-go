package staging

import (
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type HealthcheckOptions struct {
	URL          string
	ExpectedCode int
	MaxRetries   int
	Delay        time.Duration
	Timeout      time.Duration
}

func DefaultHealthcheckOptions(url string) *HealthcheckOptions {
	return &HealthcheckOptions{
		URL:          url,
		ExpectedCode: 200,
		MaxRetries:   10,
		Delay:        5 * time.Second,
		Timeout:      10 * time.Second,
	}
}

type HealthcheckResult struct {
	Passed     bool   `json:"passed"`
	StatusCode int    `json:"status_code"`
	Attempt    int    `json:"attempt"`
	Message    string `json:"message"`
}

type PageComparisonResult struct {
	Passed         bool   `json:"passed"`
	ProdURL        string `json:"prod_url"`
	RehearsalURL   string `json:"rehearsal_url"`
	ProdTitle      string `json:"prod_title"`
	RehearsalTitle string `json:"rehearsal_title"`
	Message        string `json:"message"`
}

var (
	titleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	tagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
	urlRe      = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

func CheckStagingHealth(opts *HealthcheckOptions) (*HealthcheckResult, error) {
	client := &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		resp, err := client.Get(opts.URL)
		if err != nil {
			lastErr = err
			if attempt < opts.MaxRetries {
				time.Sleep(opts.Delay)
			}
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == opts.ExpectedCode {
			return &HealthcheckResult{
				Passed:     true,
				StatusCode: resp.StatusCode,
				Attempt:    attempt,
				Message:    fmt.Sprintf("healthcheck passed on attempt %d", attempt),
			}, nil
		}

		lastErr = fmt.Errorf("unexpected status code %d (expected %d)", resp.StatusCode, opts.ExpectedCode)
		if attempt < opts.MaxRetries {
			time.Sleep(opts.Delay)
		}
	}

	return &HealthcheckResult{
		Passed:  false,
		Attempt: opts.MaxRetries,
		Message: fmt.Sprintf("healthcheck failed after %d attempts: %v", opts.MaxRetries, lastErr),
	}, lastErr
}

func CompareStagingPages(prodURL, rehearsalURL string, opts *HealthcheckOptions) (*PageComparisonResult, error) {
	if prodURL == "" || rehearsalURL == "" {
		return nil, fmt.Errorf("production and rehearsal URLs are required")
	}
	if opts == nil {
		opts = DefaultHealthcheckOptions(rehearsalURL)
	}
	client := &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	prod, err := fetchNormalizedPage(client, prodURL, opts.MaxRetries, opts.Delay)
	if err != nil {
		return &PageComparisonResult{Passed: false, ProdURL: prodURL, RehearsalURL: rehearsalURL, Message: fmt.Sprintf("failed to fetch production page: %v", err)}, err
	}
	rehearsal, err := fetchNormalizedPage(client, rehearsalURL, opts.MaxRetries, opts.Delay)
	if err != nil {
		return &PageComparisonResult{Passed: false, ProdURL: prodURL, RehearsalURL: rehearsalURL, Message: fmt.Sprintf("failed to fetch rehearsal page: %v", err)}, err
	}

	result := &PageComparisonResult{
		ProdURL:        prodURL,
		RehearsalURL:   rehearsalURL,
		ProdTitle:      prod.Title,
		RehearsalTitle: rehearsal.Title,
	}

	if prod.Title == rehearsal.Title && prod.Body == rehearsal.Body {
		result.Passed = true
		result.Message = "homepage content matches after normalization"
		return result, nil
	}

	result.Passed = false
	var reasons []string
	if prod.Title != rehearsal.Title {
		reasons = append(reasons, fmt.Sprintf("title differs: %q vs %q", prod.Title, rehearsal.Title))
	}
	if prod.Body != rehearsal.Body {
		reasons = append(reasons, "normalized body content differs")
	}
	result.Message = strings.Join(reasons, "; ")
	if result.Message == "" {
		result.Message = "homepage content differs"
	}
	return result, nil
}

type normalizedPage struct {
	Title string
	Body  string
}

func fetchNormalizedPage(client *http.Client, pageURL string, maxRetries int, delay time.Duration) (*normalizedPage, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if delay < 0 {
		delay = 0
	}
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.Get(pageURL)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				time.Sleep(delay)
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxRetries {
				time.Sleep(delay)
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			if attempt < maxRetries {
				time.Sleep(delay)
			}
			continue
		}
		title, normalized := normalizePageContent(string(body))
		return &normalizedPage{Title: title, Body: normalized}, nil
	}
	return nil, fmt.Errorf("failed to fetch %s after %d attempts: %w", pageURL, maxRetries, lastErr)
}

func normalizePageContent(raw string) (string, string) {
	raw = scriptRe.ReplaceAllString(raw, " ")
	raw = styleRe.ReplaceAllString(raw, " ")
	raw = noscriptRe.ReplaceAllString(raw, " ")
	title := ""
	if matches := titleRe.FindStringSubmatch(raw); len(matches) > 1 {
		title = html.UnescapeString(matches[1])
	}
	raw = urlRe.ReplaceAllString(raw, " <url> ")
	raw = tagRe.ReplaceAllString(raw, " ")
	raw = html.UnescapeString(raw)
	title = strings.Join(strings.Fields(strings.ToLower(title)), " ")
	body := strings.Join(strings.Fields(strings.ToLower(raw)), " ")
	return title, body
}
