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

const userAgent = "crossover-dynasty/0.1"

// Client fetches JSON from the public feeds, spacing requests to each host
// and handing every response body to Save before it is parsed. A host that
// refuses a request is left alone for CoolOff: these are free feeds, and
// asking again straight away is how an address gets shut out for longer.
type Client struct {
	HTTP    *http.Client
	Gap     time.Duration                                      // minimum time between requests to one host
	CoolOff time.Duration                                      // how long a host that refused is left alone
	Save    func(ctx context.Context, url string, body []byte) // optional
	// Via sends one host's requests to a relay instead: host -> the relay's
	// origin, e.g. "statsapi.mlb.com" -> "https://mlb.example.workers.dev".
	// The relay must forward the path and query unchanged. It is for a feed
	// that turns away the server's own address.
	Via map[string]string

	mu      sync.Mutex
	next    map[string]time.Time // host -> earliest time of its next request
	refused map[string]time.Time // host -> when it may be asked again
}

func NewClient(gap time.Duration) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Gap:     gap,
		CoolOff: 30 * time.Minute,
		next:    map[string]time.Time{},
		refused: map[string]time.Time{},
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
// connection or a brief outage, but not a host that has refused.
func (c *Client) fetch(ctx context.Context, rawURL string, v any, save bool) error {
	body, err := c.get(ctx, rawURL)
	if err != nil && ctx.Err() == nil && !Refused(err) {
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
	if until := c.refusedUntil(u.Host); time.Now().Before(until) {
		return nil, &StatusError{URL: rawURL, Code: http.StatusTooManyRequests, Until: until}
	}
	if err := c.wait(ctx, u.Host); err != nil {
		return nil, err
	}

	target := rawURL
	if relay := c.Via[u.Host]; relay != "" {
		target = relay + u.RequestURI()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
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
		status := &StatusError{URL: rawURL, Code: res.StatusCode}
		if Refused(status) {
			status.Until = time.Now().Add(c.CoolOff)
			c.mu.Lock()
			c.refused[u.Host] = status.Until
			c.mu.Unlock()
		}
		return nil, status
	}
	return body, nil
}

func (c *Client) refusedUntil(host string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refused[host]
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
	URL   string
	Code  int
	Until time.Time // set when the host refused: nothing is sent to it before then
}

func (e *StatusError) Error() string {
	text := fmt.Sprintf("GET %s: %d %s", e.URL, e.Code, http.StatusText(e.Code))
	if !e.Until.IsZero() {
		text += "; leaving this feed alone until " + e.Until.Format("15:04")
	}
	return text
}

// Refused reports whether err is a feed turning the request away as
// unwelcome (blocked or rate limited), as opposed to failing or lacking
// the page. More requests will not help and may make it worse.
func Refused(err error) bool {
	var status *StatusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.Code {
	case http.StatusForbidden, http.StatusNotAcceptable, http.StatusTooManyRequests:
		return true
	}
	return false
}

// NotFound reports whether err is a feed answering 404.
func NotFound(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == http.StatusNotFound
}
