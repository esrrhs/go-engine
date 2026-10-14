package tools

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWeb_FetchTransportPinsValidatedIPs guards the wiring of the
// DNS-rebinding defense: the client used by web_fetch / http_request /
// download_file must run its connections through a policy-checking dialer,
// not the default transport. A refactor that drops the custom transport
// reopens the validateURL→Do TOCTOU window, so this must fail loudly.
func TestWeb_FetchTransportPinsValidatedIPs(t *testing.T) {
	w := newTestWeb(t, "", false)
	tr, ok := w.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("fetch client must use an explicit *http.Transport, got %T", w.client.Transport)
	}
	if tr.DialContext == nil {
		t.Fatal("fetch transport must set DialContext to the policy-checking dialer")
	}
}

// TestWeb_DialPolicyBlocksLoopbackAtConnectTime pins the enforcement point
// itself. A rebinding host can serve a public A record when the URL is
// validated and a loopback A record when the transport dials seconds later;
// only a check inside the dialer closes that window. The search endpoint is
// operator-trusted and deliberately exempt (see searchClient).
func TestWeb_DialPolicyBlocksLoopbackAtConnectTime(t *testing.T) {
	w := newTestWeb(t, "", false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.secureDialContext(ctx, "tcp", "localhost:1")
	if err == nil {
		t.Fatal("dialer must refuse a loopback address even mid-request")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("expected an address-policy error, got %v", err)
	}

	// AllowLocal skips the policy entirely: the same dial must now fail only
	// with a connection error (nothing listens on port 1), proving the
	// policy — not the dialer — is what refuses the address above.
	w.AllowLocal = true
	if _, err := w.secureDialContext(ctx, "tcp", "localhost:1"); err == nil {
		t.Fatal("expected connection refused on a closed port")
	} else if strings.Contains(err.Error(), "non-public") {
		t.Fatalf("AllowLocal must skip the IP policy, got %v", err)
	}
}

// TestWeb_FetchLoopbackBlockedEndToEnd runs the whole Fetch path against a
// real loopback server: blocked under the default policy, successful with
// AllowLocal (which is how tests and trusted setups point at local
// endpoints). Guards both the block and against over-blocking.
func TestWeb_FetchLoopbackBlockedEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	w := newTestWeb(t, "", false)
	if _, err := w.Fetch(context.Background(), srv.URL+"/x"); err == nil {
		t.Fatal("loopback fetch must fail when AllowLocal is off")
	}

	w.AllowLocal = true
	out, err := w.Fetch(context.Background(), srv.URL+"/x")
	if err != nil {
		t.Fatalf("AllowLocal fetch must succeed: %v", err)
	}
	if !strings.Contains(out, "payload") {
		t.Fatalf("unexpected body: %q", out)
	}
}

// TestWeb_SearchEndpointTrustedLikeAllowLocal locks in the documented
// trust model: the operator-configured search endpoint (e.g. an internal
// SearXNG) may resolve to a private address, so it must not go through the
// policy-checking transport.
func TestWeb_SearchEndpointTrustedLikeAllowLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ddgSample))
	}))
	defer srv.Close()

	w := newTestWeb(t, srv.URL+"/?q={query}", false) // AllowLocal off
	out, err := w.Search(context.Background(), "go", 5)
	if err != nil {
		t.Fatalf("operator search endpoint must be trusted regardless of AllowLocal: %v", err)
	}
	if !strings.Contains(out, "The Go Programming Language") {
		t.Fatalf("search via trusted endpoint failed:\n%s", out)
	}
	if _, ok := w.searchClient.Transport.(*http.Transport); ok {
		t.Fatal("search client must not use the policy transport")
	}
}

// TestIsPublicIP_NonGlobalRanges covers ranges that parse fine but are not
// globally routable: reaching them from a fetch is either an SSRF against
// internal infrastructure (CGNAT, NAT64-encoded private targets) or at best
// a black hole. Go's net.IP predicates alone do not cover these.
func TestIsPublicIP_NonGlobalRanges(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "0.1.2.3", // 0.0.0.0/8 "this network"
		"100.64.0.1", "100.127.255.255", // 100.64.0.0/10 CGNAT
		"192.0.0.1", "192.0.2.1", // 192.0.0.0/24 + TEST-NET-1
		"192.88.99.1",                  // 6to4 relay anycast
		"198.18.0.1", "198.19.255.255", // 198.18.0.0/15 benchmarking
		"198.51.100.7",                 // TEST-NET-2
		"203.0.113.9",                  // TEST-NET-3
		"240.0.0.1", "255.255.255.255", // 240.0.0.0/4 reserved + broadcast
		"64:ff9b::7f00:1",                                    // NAT64 well-known prefix (encodes 127.0.0.1)
		"fec0::1",                                            // fec0::/10 site-local (deprecated)
		"2001:db8::1",                                        // documentation
		"127.0.0.1", "10.0.0.1", "192.168.1.1", "172.16.0.1", // loopback/private
		"169.254.1.1", "::1", "fe80::1", "fc00::1", "::", "224.0.0.1", // link-local/ULA/multicast
	}
	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34",
		"100.63.255.255", "100.128.0.1", // CGNAT boundaries: just outside
		"192.0.1.1", "198.51.99.1", "223.255.255.255", // reserved-range boundaries
		"2606:2800:220:1:248:1893:25c8:1946", "2001:4860:4860::8888",
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test bug: %q did not parse", s)
		}
		if isPublicIP(ip) {
			t.Errorf("isPublicIP(%s) = true; want false", s)
		}
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test bug: %q did not parse", s)
		}
		if !isPublicIP(ip) {
			t.Errorf("isPublicIP(%s) = false; want true", s)
		}
	}
}
