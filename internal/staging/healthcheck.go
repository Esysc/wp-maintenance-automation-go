package staging

import (
	"crypto/tls"
	"fmt"
	"net/http"
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
