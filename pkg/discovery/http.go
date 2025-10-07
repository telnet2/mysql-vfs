package discovery

import (
	"context"
	"net"
	"net/http"
	"time"

	consulhttp "code.byted.org/gopkg/consul/http"
)

// NewHTTPClient creates an HTTP client with Consul service discovery support
// This client can handle regular URLs and consul+ URLs transparently
func NewHTTPClient(opts ...consulhttp.ClientOption) *http.Client {
	// Use the consul HTTP transport which handles service discovery automatically
	transport := consulhttp.NewTransport(opts...)

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// NewHTTPTransport creates an HTTP transport with Consul service discovery
// Use this if you need to customize the transport further
func NewHTTPTransport(opts ...consulhttp.ClientOption) *http.Transport {
	return consulhttp.NewTransport(opts...)
}

// NewHTTPTransportWithConsulResolver creates a custom HTTP transport
// that resolves consul+ URLs before making requests
func NewHTTPTransportWithConsulResolver() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
}

// ConsulHTTPClient wraps the consul http client for easy use
type ConsulHTTPClient struct {
	*consulhttp.HttpClient
}

// NewConsulHTTPClient creates a new HTTP client with Consul service discovery
// This uses the gopkg/consul/http client which provides:
// - Automatic service name resolution (e.g., http://my-service/api)
// - Load balancing across endpoints
// - Connection pooling
func NewConsulHTTPClient(opts ...consulhttp.ClientOption) *ConsulHTTPClient {
	return &ConsulHTTPClient{
		HttpClient: consulhttp.NewHttpClient(opts...),
	}
}
