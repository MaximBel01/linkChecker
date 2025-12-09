package checker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// StatusResult represents the result of checking a single URL
type StatusResult struct {
	URL       string `json:"url"`
	Status    int    `json:"status"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
}

// Common error types for better error handling
var (
	ErrEmptyURL          = errors.New("URL cannot be empty")
	ErrInvalidURL        = errors.New("invalid URL format")
	ErrUnsupportedScheme = errors.New("unsupported URL scheme")
	ErrTimeout           = errors.New("request timeout")
	ErrDNS               = errors.New("DNS resolution failed")
	ErrConnection        = errors.New("connection failed")
)

// LinkChecker handles checking URL availability with comprehensive error handling
type LinkChecker struct {
	client  *http.Client
	timeout time.Duration
}

// NewLinkChecker creates a new LinkChecker with validation
func NewLinkChecker(timeout time.Duration) (*LinkChecker, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive, got %v", timeout)
	}
	if timeout > 5*time.Minute {
		return nil, fmt.Errorf("timeout too large: %v", timeout)
	}

	// Create custom transport with better error handling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false,
		DisableKeepAlives:     false,
		ResponseHeaderTimeout: timeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects to prevent infinite loops
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}

	return &LinkChecker{
		client:  client,
		timeout: timeout,
	}, nil
}

// CheckLinks checks multiple URLs concurrently with comprehensive error handling
func (lc *LinkChecker) CheckLinks(urls []string) []StatusResult {
	if urls == nil {
		return []StatusResult{{
			URL:       "",
			Status:    0,
			Available: false,
			Error:     "input slice cannot be nil",
			CheckedAt: time.Now().Format(time.RFC3339),
		}}
	}

	if len(urls) == 0 {
		return []StatusResult{}
	}

	results := make([]StatusResult, len(urls))
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		index  int
		result StatusResult
	})

	// Limit concurrent requests to prevent resource exhaustion
	semaphore := make(chan struct{}, 100) // Max 100 concurrent requests

	for i, rawURL := range urls {
		wg.Add(1)
		go func(index int, u string) {
			// Use semaphore to limit concurrency
			semaphore <- struct{}{}
			defer func() {
				<-semaphore
				wg.Done()
			}()

			// Protect against panics in goroutine
			defer func() {
				if r := recover(); r != nil {
					resultsChan <- struct {
						index  int
						result StatusResult
					}{
						index,
						StatusResult{
							URL:       u,
							Status:    0,
							Available: false,
							Error:     fmt.Sprintf("panic during check: %v", r),
							CheckedAt: time.Now().Format(time.RFC3339),
						},
					}
				}
			}()

			result := lc.checkURL(u)
			resultsChan <- struct {
				index  int
				result StatusResult
			}{index, result}
		}(i, rawURL)
	}

	// Wait for all goroutines to complete and close channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for r := range resultsChan {
		results[r.index] = r.result
	}

	return results
}

// checkURL performs comprehensive URL checking with detailed error handling
func (lc *LinkChecker) checkURL(rawURL string) StatusResult {
	result := StatusResult{
		URL:       rawURL,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	// Basic validation
	if strings.TrimSpace(rawURL) == "" {
		result.Error = ErrEmptyURL.Error()
		result.Available = false
		return result
	}

	// Parse and validate URL
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrInvalidURL, err).Error()
		result.Available = false
		return result
	}

	// Check URL scheme
	if parsedURL.Scheme == "" {
		result.Error = fmt.Errorf("%w: missing scheme (http/https)", ErrInvalidURL).Error()
		result.Available = false
		return result
	}

	if !lc.isSupportedScheme(parsedURL.Scheme) {
		result.Error = fmt.Errorf("%w: %s", ErrUnsupportedScheme, parsedURL.Scheme).Error()
		result.Available = false
		return result
	}

	// Validate host
	if parsedURL.Host == "" {
		result.Error = fmt.Errorf("%w: missing host", ErrInvalidURL).Error()
		result.Available = false
		return result
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), lc.timeout)
	defer cancel()

	// Try HEAD request first, fallback to GET
	var req *http.Request
	var requestErr error

	// HEAD request
	req, requestErr = http.NewRequestWithContext(ctx, "HEAD", parsedURL.String(), nil)
	if requestErr != nil {
		// Try GET as fallback
		req, requestErr = http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
		if requestErr != nil {
			result.Error = fmt.Errorf("%w: %v", ErrInvalidURL, requestErr).Error()
			result.Available = false
			return result
		}
	}

	// Set reasonable headers
	req.Header.Set("User-Agent", "LinkChecker/1.0")
	req.Header.Set("Accept", "*/*")

	// Execute request with detailed error handling
	resp, err := lc.client.Do(req)

	// Handle different types of errors
	if err != nil {
		result.Error = lc.classifyError(err, ctx.Err()).Error()
		result.Available = false
		return result
	}

	// Ensure response body is closed
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	// Check for specific HTTP status codes
	result.Status = resp.StatusCode

	// Determine availability based on status code
	result.Available = resp.StatusCode >= 200 && resp.StatusCode < 400

	// Handle redirects and client errors
	if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return result
}

// isSupportedScheme checks if the URL scheme is supported
func (lc *LinkChecker) isSupportedScheme(scheme string) bool {
	supportedSchemes := map[string]bool{
		"http":  true,
		"https": true,
	}
	return supportedSchemes[strings.ToLower(scheme)]
}

// classifyError provides detailed error classification
func (lc *LinkChecker) classifyError(err error, ctxErr error) error {
	if ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrTimeout, ctxErr)
		}
		if errors.Is(ctxErr, context.Canceled) {
			return fmt.Errorf("request cancelled: %v", ctxErr)
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("%w: %v", ErrTimeout, netErr)
		}
		// Note: netErr.Temporary() was deprecated in Go 1.18
		// Most temporary errors are timeouts, which are handled above
		// For other network issues, we'll classify them as general connection errors
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("%w: %v", ErrDNS, dnsErr)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("%w: %v", ErrConnection, opErr)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if strings.Contains(urlErr.Error(), "unsupported protocol scheme") {
			return fmt.Errorf("%w: %v", ErrUnsupportedScheme, urlErr)
		}
		return fmt.Errorf("URL error: %v", urlErr)
	}

	// For other types of errors, wrap with generic connection error
	return fmt.Errorf("%w: %v", ErrConnection, err)
}
