package checker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type StatusResult struct {
	URL       string `json:"url"`
	Status    int    `json:"status"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
}

type LinkChecker struct {
	client  *http.Client
	timeout time.Duration
}

func NewLinkChecker(timeout time.Duration) *LinkChecker {
	return &LinkChecker{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

func (lc *LinkChecker) CheckLinks(urls []string) []StatusResult {
	results := make([]StatusResult, len(urls))
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		index  int
		result StatusResult
	})

	for i, url := range urls {
		wg.Add(1)
		go func(index int, u string) {
			defer wg.Done()
			result := lc.checkURL(u)
			resultsChan <- struct {
				index  int
				result StatusResult
			}{index, result}
		}(i, url)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for r := range resultsChan {
		results[r.index] = r.result
	}

	return results
}

func (lc *LinkChecker) checkURL(url string) StatusResult {
	result := StatusResult{
		URL:       url,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	if url == "" {
		result.Error = "empty URL"
		result.Available = false
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), lc.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			result.Error = fmt.Sprintf("invalid URL: %v", err)
			result.Available = false
			return result
		}
	}

	resp, err := lc.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("connection error: %v", err)
		result.Available = false
		return result
	}
	defer resp.Body.Close()

	result.Status = resp.StatusCode
	result.Available = resp.StatusCode >= 200 && resp.StatusCode < 400

	return result
}
