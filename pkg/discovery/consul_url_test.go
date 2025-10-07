package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsulURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantErr     bool
		wantScheme  string
		wantService string
		wantPath    string
		wantIDC     string
		wantCluster string
		wantLimit   int
	}{
		{
			name:        "basic nats URL",
			url:         "consul+nats://nats-service",
			wantScheme:  "nats",
			wantService: "nats-service",
			wantPath:    "",
			wantLimit:   10,
		},
		{
			name:        "https with path",
			url:         "consul+https://api-service/v1/users",
			wantScheme:  "https",
			wantService: "api-service",
			wantPath:    "/v1/users",
			wantLimit:   10,
		},
		{
			name:        "with consul options",
			url:         "consul+nats://nats-cluster?consul.idc=lf&consul.cluster=prod&consul.limit=20",
			wantScheme:  "nats",
			wantService: "nats-cluster",
			wantPath:    "",
			wantIDC:     "lf",
			wantCluster: "prod",
			wantLimit:   20,
		},
		{
			name:        "with query params and consul options",
			url:         "consul+http://api-service/search?query=test&consul.cluster=prod",
			wantScheme:  "http",
			wantService: "api-service",
			wantPath:    "/search",
			wantCluster: "prod",
			wantLimit:   10,
		},
		{
			name:        "mysql URL",
			url:         "consul+mysql://mysql-db/mydb",
			wantScheme:  "mysql",
			wantService: "mysql-db",
			wantPath:    "/mydb",
			wantLimit:   10,
		},
		{
			name:    "missing consul+ prefix",
			url:     "nats://nats-service",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			url:     "consul+",
			wantErr: true,
		},
		{
			name:    "missing service name",
			url:     "consul+nats://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cu, err := ParseConsulURL(tt.url)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantScheme, cu.Scheme)
			assert.Equal(t, tt.wantService, cu.ServiceName)
			assert.Equal(t, tt.wantPath, cu.Path)
			assert.Equal(t, tt.wantIDC, cu.Options.IDC)
			assert.Equal(t, tt.wantCluster, cu.Options.Cluster)
			assert.Equal(t, tt.wantLimit, cu.Options.Limit)

			// Verify consul.* params are removed from query
			for key := range cu.Query {
				assert.False(t, contains(key, "consul."), "consul.* params should be removed from query")
			}
		})
	}
}

func TestIsConsulURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"consul nats URL", "consul+nats://nats-service", true},
		{"consul https URL", "consul+https://api-service", true},
		{"regular nats URL", "nats://localhost:4222", false},
		{"regular https URL", "https://api.example.com", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConsulURL(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveOrPassthrough(t *testing.T) {
	t.Run("passthrough regular URL", func(t *testing.T) {
		regularURL := "nats://localhost:4222"
		resolved, err := ResolveOrPassthrough(regularURL)
		require.NoError(t, err)
		assert.Equal(t, regularURL, resolved)
	})

	t.Run("passthrough https URL", func(t *testing.T) {
		httpsURL := "https://api.example.com/v1/users"
		resolved, err := ResolveOrPassthrough(httpsURL)
		require.NoError(t, err)
		assert.Equal(t, httpsURL, resolved)
	})

	// Note: Actual consul lookup tests would require a running Consul agent
	// For unit tests, we just verify the parsing and structure
}

func TestExtractConsulOptions(t *testing.T) {
	tests := []struct {
		name        string
		query       map[string][]string
		wantIDC     string
		wantCluster string
		wantLimit   int
	}{
		{
			name: "all options",
			query: map[string][]string{
				"consul.idc":     {"lf"},
				"consul.cluster": {"prod"},
				"consul.limit":   {"50"},
			},
			wantIDC:     "lf",
			wantCluster: "prod",
			wantLimit:   50,
		},
		{
			name: "partial options",
			query: map[string][]string{
				"consul.cluster": {"staging"},
			},
			wantIDC:     "",
			wantCluster: "staging",
			wantLimit:   10, // default
		},
		{
			name:        "no options",
			query:       map[string][]string{},
			wantIDC:     "",
			wantCluster: "",
			wantLimit:   10, // default
		},
		{
			name: "invalid limit",
			query: map[string][]string{
				"consul.limit": {"invalid"},
			},
			wantIDC:     "",
			wantCluster: "",
			wantLimit:   10, // default (parsing fails)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := extractConsulOptions(tt.query)
			assert.Equal(t, tt.wantIDC, opts.IDC)
			assert.Equal(t, tt.wantCluster, opts.Cluster)
			assert.Equal(t, tt.wantLimit, opts.Limit)
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
