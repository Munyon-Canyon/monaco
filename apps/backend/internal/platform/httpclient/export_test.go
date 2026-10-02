package httpclient

import (
	"net/http"
	"time"
)

func WithFullDelay() Option {
	return func(c *Client) { c.jitter = func(d time.Duration) time.Duration { return d } }
}

func RoundTripper(c *Client) http.RoundTripper { return c.http.Transport }
