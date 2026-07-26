package healthcheck

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Checker struct {
	client *http.Client
}

type DefaultOptions struct {
	URL          string
	ExpectedCode int
	MaxRetries   int
	Timeout      time.Duration
	Insecure     bool
}

func NewDefaultOptions(url string) *DefaultOptions {
	return &DefaultOptions{
		URL:          url,
		ExpectedCode: 200,
		MaxRetries:   5,
		Timeout:      30 * time.Second,
		Insecure:     false,
	}
}

func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Checker) Check(opts *DefaultOptions) (map[string]interface{}, error) {
	if opts.URL == "" {
		return nil, fmt.Errorf("healthcheck URL is required")
	}

	var lastError error

	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		result, err := c.checkOnce(context.Background(), opts)
		if err != nil {
			lastError = err
			if attempt < opts.MaxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}

		return result, nil
	}

	return nil, fmt.Errorf("healthcheck failed after %d attempts: %v", opts.MaxRetries, lastError)
}

func (c *Checker) checkOnce(ctx context.Context, opts *DefaultOptions) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", opts.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if opts.Insecure {
		c.client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	} else {
		c.client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		}
	}

	startTime := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseTime := time.Since(startTime).Milliseconds()
	passed := resp.StatusCode == opts.ExpectedCode

	return map[string]interface{}{
		"url":            opts.URL,
		"status_code":    resp.StatusCode,
		"expected_code":  opts.ExpectedCode,
		"passed":         passed,
		"attempt":        1,
		"max_attempts":   opts.MaxRetries,
		"response_time_ms": responseTime,
		"error_message":  "",
	}, nil
}

func (c *Checker) QuickCheck(url string) (bool, error) {
	opts := NewDefaultOptions(url)
	result, err := c.Check(opts)
	if err != nil {
		return false, err
	}

	return result["passed"].(bool), nil
}

func (c *Checker) Ping(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	defer conn.Close()

	return nil
}