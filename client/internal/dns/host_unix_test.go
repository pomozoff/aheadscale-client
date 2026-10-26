//go:build (linux && !android) || freebsd

package dns

import (
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseNsswitchResolveAhead(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "resolve before dns with action token",
			in:   "hosts: mymachines resolve [!UNAVAIL=return] files myhostname dns\n",
			want: true,
		},
		{
			name: "dns before resolve",
			in:   "hosts: files mdns4_minimal [NOTFOUND=return] dns resolve\n",
			want: false,
		},
		{
			name: "debian default with only dns",
			in:   "hosts: files mdns4_minimal [NOTFOUND=return] dns mymachines\n",
			want: false,
		},
		{
			name: "neither resolve nor dns",
			in:   "hosts: files myhostname\n",
			want: false,
		},
		{
			name: "no hosts line",
			in:   "passwd: files systemd\ngroup: files systemd\n",
			want: false,
		},
		{
			name: "empty",
			in:   "",
			want: false,
		},
		{
			name: "comments and blank lines ignored",
			in:   "# comment\n\n# another\nhosts: resolve dns\n",
			want: true,
		},
		{
			name: "trailing inline comment",
			in:   "hosts: resolve [!UNAVAIL=return] dns # fallback\n",
			want: true,
		},
		{
			name: "hosts token must be the first field",
			in:   "  hosts: resolve dns\n",
			want: true,
		},
		{
			name: "other db line mentioning resolve is ignored",
			in:   "networks: resolve\nhosts: dns\n",
			want: false,
		},
		{
			name: "only resolve, no dns",
			in:   "hosts: files resolve\n",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseNsswitchResolveAhead([]byte(tt.in)); got != tt.want {
				t.Errorf("parseNsswitchResolveAhead() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDesktopNetworkManagerCompatibility(t *testing.T) {
	for _, tc := range []struct {
		value     string
		supported bool
	}{
		{"1.16.0", true}, {"1.26.6", true}, {"1.28.0", false},
		{"1.44.2", true}, {"1.46.0", true}, {"1.48.10", true},
		{"1.54.3", true}, {"1.56.0", true}, {"2.0.0", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			parsed, err := parseVersion(tc.value)
			require.NoError(t, err)
			var supported bool
			for _, rule := range supportedNetworkManagerVersionConstraints {
				constraint, err := version.NewConstraint(rule)
				require.NoError(t, err)
				supported = supported || constraint.Check(parsed)
			}
			assert.Equal(t, tc.supported, supported, "desktop releases must use NetworkManager DNS without hiding the tunnel")
		})
	}
}

func TestNetworkManagerPrivateRoutingDomains(t *testing.T) {
	config := HostDNSConfig{Domains: []DomainConfig{
		{Domain: "aheadscale.internal"},
		{Domain: "dev.example", MatchOnly: true},
		{Domain: "old.example", MatchOnly: true, Disabled: true},
	}}
	search, routing := networkManagerDomainLists(config)
	assert.Equal(t, []string{"aheadscale.internal"}, search, "search domains must expand short device names")
	assert.Equal(t, []string{"~dev.example"}, routing, "routing-only domains need the NetworkManager tilde prefix without an empty DNS label")
}
