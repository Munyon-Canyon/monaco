package apns

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
)

func WithTransport(rt http.RoundTripper) Option { return func(c *Client) { c.transport = rt } }

func WithDialTLS(dial func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error)) Option {
	return func(c *Client) { c.dialTLS = dial }
}

func Retarget(c *Client, host string) {
	for _, ep := range c.endpoints {
		ep.client.Host = host
	}
}
