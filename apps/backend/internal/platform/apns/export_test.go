package apns

import "net/http"

func WithTransport(rt http.RoundTripper) Option { return func(c *Client) { c.transport = rt } }
