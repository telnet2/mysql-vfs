package discovery

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"code.byted.org/gopkg/consul"
)

// ConsulURL represents a parsed consul+ URL
type ConsulURL struct {
	OriginalURL string
	Scheme      string // original scheme without "consul+" prefix (e.g., "nats", "https", "http", "mysql")
	ServiceName string
	Path        string
	Query       url.Values
	Options     ConsulLookupOptions
}

// ConsulLookupOptions contains options for Consul service lookup
type ConsulLookupOptions struct {
	IDC     string
	Cluster string
	Limit   int
}

// ParseConsulURL parses a consul+ URL into its components
// Supported formats:
//   consul+nats://service-name
//   consul+https://service-name/path
//   consul+http://service-name/path?query=value
//   consul+mysql://service-name/dbname
//
// Consul options can be passed as query parameters:
//   consul+nats://service-name?consul.idc=lf&consul.cluster=prod&consul.limit=10
func ParseConsulURL(consulURL string) (*ConsulURL, error) {
	if !strings.HasPrefix(consulURL, "consul+") {
		return nil, fmt.Errorf("URL must start with 'consul+': %s", consulURL)
	}

	// Remove "consul+" prefix
	urlWithoutPrefix := strings.TrimPrefix(consulURL, "consul+")

	parsed, err := url.Parse(urlWithoutPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsed.Scheme == "" {
		return nil, fmt.Errorf("missing scheme after consul+: %s", consulURL)
	}

	serviceName := parsed.Host
	if serviceName == "" {
		return nil, fmt.Errorf("missing service name: %s", consulURL)
	}

	cu := &ConsulURL{
		OriginalURL: consulURL,
		Scheme:      parsed.Scheme,
		ServiceName: serviceName,
		Path:        parsed.Path,
		Query:       parsed.Query(),
	}

	// Extract Consul-specific options from query parameters
	cu.Options = extractConsulOptions(cu.Query)

	// Remove consul.* parameters from query
	cleanQuery := url.Values{}
	for k, v := range cu.Query {
		if !strings.HasPrefix(k, "consul.") {
			cleanQuery[k] = v
		}
	}
	cu.Query = cleanQuery

	return cu, nil
}

// extractConsulOptions extracts consul.* query parameters
func extractConsulOptions(query url.Values) ConsulLookupOptions {
	opts := ConsulLookupOptions{
		Limit: 10, // default
	}

	if idc := query.Get("consul.idc"); idc != "" {
		opts.IDC = idc
	}
	if cluster := query.Get("consul.cluster"); cluster != "" {
		opts.Cluster = cluster
	}
	if limit := query.Get("consul.limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			opts.Limit = l
		}
	}

	return opts
}

// ResolveConsulURL resolves a consul+ URL to a concrete endpoint URL
// Returns the resolved URL with an actual host:port from Consul
func ResolveConsulURL(consulURL string) (string, error) {
	cu, err := ParseConsulURL(consulURL)
	if err != nil {
		return "", err
	}

	return cu.Resolve()
}

// Resolve performs Consul lookup and returns a concrete URL with host:port
func (cu *ConsulURL) Resolve() (string, error) {
	// Build Consul lookup options
	lookupOpts := []consul.LookupOptions{}

	if cu.Options.IDC != "" {
		lookupOpts = append(lookupOpts, consul.WithIDC(consul.IDC(cu.Options.IDC)))
	}
	if cu.Options.Cluster != "" {
		lookupOpts = append(lookupOpts, consul.WithCluster(cu.Options.Cluster))
	}
	if cu.Options.Limit > 0 {
		lookupOpts = append(lookupOpts, consul.WithLimit(cu.Options.Limit))
	}

	// Lookup service in Consul
	endpoints, err := consul.Lookup(cu.ServiceName, lookupOpts...)
	if err != nil {
		return "", fmt.Errorf("consul lookup failed for %s: %w", cu.ServiceName, err)
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("no endpoints found for service: %s", cu.ServiceName)
	}

	// Get one endpoint (weighted random selection)
	endpoint := endpoints.GetOne()

	// Build the final URL
	finalURL := &url.URL{
		Scheme: cu.Scheme,
		Host:   endpoint.Addr,
		Path:   cu.Path,
	}

	if len(cu.Query) > 0 {
		finalURL.RawQuery = cu.Query.Encode()
	}

	return finalURL.String(), nil
}

// MustResolveConsulURL is like ResolveConsulURL but panics on error
func MustResolveConsulURL(consulURL string) string {
	resolved, err := ResolveConsulURL(consulURL)
	if err != nil {
		panic(fmt.Sprintf("failed to resolve consul URL %s: %v", consulURL, err))
	}
	return resolved
}

// IsConsulURL checks if a URL starts with "consul+"
func IsConsulURL(urlStr string) bool {
	return strings.HasPrefix(urlStr, "consul+")
}

// ResolveOrPassthrough resolves a consul+ URL, or returns the original if not a consul URL
func ResolveOrPassthrough(urlStr string) (string, error) {
	if IsConsulURL(urlStr) {
		return ResolveConsulURL(urlStr)
	}
	return urlStr, nil
}
