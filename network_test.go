package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func mustParseCIDR(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return network
}

func TestIpAllowed(t *testing.T) {
	cases := []struct {
		name     string
		networks []*net.IPNet
		ip       string
		want     bool
	}{
		{
			name:     "no networks configured allows all",
			networks: nil,
			ip:       "1.2.3.4",
			want:     true,
		},
		{
			name:     "IPv4 in range",
			networks: []*net.IPNet{mustParseCIDR("192.168.1.0/24")},
			ip:       "192.168.1.100",
			want:     true,
		},
		{
			name:     "IPv4 outside range",
			networks: []*net.IPNet{mustParseCIDR("192.168.1.0/24")},
			ip:       "10.0.0.1",
			want:     false,
		},
		{
			name:     "IPv6 in range",
			networks: []*net.IPNet{mustParseCIDR("fd00::/8")},
			ip:       "fd12:3456:789a::1",
			want:     true,
		},
		{
			name:     "IPv6 outside range",
			networks: []*net.IPNet{mustParseCIDR("fd00::/8")},
			ip:       "2001:db8::1",
			want:     false,
		},
		{
			name:     "unparseable IP string",
			networks: []*net.IPNet{mustParseCIDR("192.168.1.0/24")},
			ip:       "not-an-ip",
			want:     false,
		},
		{
			name: "multiple networks, first matches",
			networks: []*net.IPNet{
				mustParseCIDR("192.168.1.0/24"),
				mustParseCIDR("10.0.0.0/8"),
			},
			ip:   "192.168.1.5",
			want: true,
		},
		{
			name: "multiple networks, second matches",
			networks: []*net.IPNet{
				mustParseCIDR("192.168.1.0/24"),
				mustParseCIDR("10.0.0.0/8"),
			},
			ip:   "10.5.5.5",
			want: true,
		},
		{
			name: "multiple networks, none match",
			networks: []*net.IPNet{
				mustParseCIDR("192.168.1.0/24"),
				mustParseCIDR("10.0.0.0/8"),
			},
			ip:   "172.16.0.1",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{Config: &Config{AllowedNetworks: tc.networks}}
			got := app.ipAllowed(tc.ip)
			if got != tc.want {
				t.Errorf("ipAllowed(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestConfigAllowedNetworksParsing(t *testing.T) {
	valid := []string{"192.168.1.0/24", "10.0.0.0/8", "fd00::/8", "::1/128"}
	for _, cidr := range valid {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Errorf("expected %q to be valid CIDR, got: %v", cidr, err)
		}
	}

	invalid := []string{"notacidr", "256.0.0.1/24", "192.168.1.0"}
	for _, cidr := range invalid {
		_, _, err := net.ParseCIDR(cidr)
		if err == nil {
			t.Errorf("expected %q to be invalid CIDR, but got no error", cidr)
		}
	}
}

func TestDashboardNetworkGuard(t *testing.T) {
	doors := []DoorConfig{{Key: "lock_front", Name: "Front Door", EntityID: "lock.front"}}
	// HA stub not needed: network check short-circuits before any HA call.
	ha := newFakeHA(fakeHAConfig{})
	defer ha.Close()

	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")
	app.Config.AllowedNetworks = []*net.IPNet{mustParseCIDR("127.0.0.0/8")}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	w := httptest.NewRecorder()

	app.DashboardHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Please join the WiFi") {
		t.Errorf("body does not contain 'Please join the WiFi':\n%s", w.Body.String())
	}
}

func TestDashboardAllowedNetwork(t *testing.T) {
	doors := []DoorConfig{{Key: "lock_front", Name: "Front Door", EntityID: "lock.front"}}
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_boolean.allowance": "on",
	}})
	defer ha.Close()

	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")
	app.Config.AllowedNetworks = []*net.IPNet{mustParseCIDR("127.0.0.0/8")}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	app.DashboardHandler(w, req)

	if strings.Contains(w.Body.String(), "Please join the WiFi") {
		t.Errorf("body should not contain 'Please join the WiFi' for an allowed IP")
	}
}

func TestOpenDoorNetworkGuard(t *testing.T) {
	doors := []DoorConfig{{Key: "lock_front", Name: "Front Door", EntityID: "lock.front"}}
	// HA stub not needed: network check short-circuits before any HA call.
	ha := newFakeHA(fakeHAConfig{})
	defer ha.Close()

	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")
	app.Config.AllowedNetworks = []*net.IPNet{mustParseCIDR("127.0.0.0/8")}

	form := url.Values{"door": {"lock_front"}}
	req := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	w := httptest.NewRecorder()

	app.OpenDoorHandler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}
