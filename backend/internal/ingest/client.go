package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const userAgent = "crossover-dynasty/0.1 (hobby fantasy league)"

// Client fetches JSON from the public feeds, spacing requests to each host
// and handing every response body to Save before it is parsed.
type Client struct {
	HTTP *http.Client
	Gap  time.Duration                                      // minimum time between requests to one host
	Save func(ctx context.Context, url string, body []byte) // optional

	mu   sync.Mutex
	next map[string]time.Time // host -> earliest time of its next request
}

func NewClient(gap time.Duration) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 30 * time.Second},
		Gap:  gap,
		next: map[string]time.Time{},
	}
}

// GetJSON fetches rawURL, hands the body to Save and decodes it into v. Use
// it for feeds whose URLs are few and stable, such as rosters.
func (c *Client) GetJSON(ctx context.Context, rawURL string, v any) error {
	return c.fetch(ctx, rawURL, v, true)
}

// GetTransient is GetJSON without saving the body. Use it for feeds with a
// new URL per day or per game, which would otherwise pile up forever.
func (c *Client) GetTransient(ctx context.Context, rawURL string, v any) error {
	return c.fetch(ctx, rawURL, v, false)
}

// fetch tries a failed request once more, which covers a dropped
// connection or a brief outage.
func (c *Client) fetch(ctx context.Context, rawURL string, v any, save bool) error {
	body, err := c.get(ctx, rawURL)
	if err != nil && ctx.Err() == nil {
		body, err = c.get(ctx, rawURL)
	}
	if err != nil {
		return err
	}
	if save && c.Save != nil {
		c.Save(ctx, rawURL, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("GET %s: decode: %w", rawURL, err)
	}
	return nil
}

// get makes one rate-limited request and returns the body of a 200 response.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if err := c.wait(ctx, u.Host); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, &StatusError{URL: rawURL, Code: res.StatusCode}
	}
	return body, nil
}

// wait reserves the next request slot for host and sleeps until it arrives.
func (c *Client) wait(ctx context.Context, host string) error {
	c.mu.Lock()
	now := time.Now()
	slot := c.next[host]
	if slot.Before(now) {
		slot = now
	}
	c.next[host] = slot.Add(c.Gap)
	c.mu.Unlock()

	select {
	case <-time.After(slot.Sub(now)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StatusError is a response other than 200. Callers that expect a feed to
// be missing sometimes (a list not published yet) can check Code.
type StatusError struct {
	URL  string
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: %d %s", e.URL, e.Code, http.StatusText(e.Code))
}

// NotFound reports whether err is a feed answering 404.
func NotFound(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == http.StatusNotFound
}
