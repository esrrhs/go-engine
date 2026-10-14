package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// defaultSearchURL is the keyless DuckDuckGo lite endpoint. Result anchors
// are duckduckgo.com redirect links whose real target is the uddg parameter.
const defaultSearchURL = "https://lite.duckduckgo.com/lite/?q={query}"

const (
	defaultWebTimeout = 30 * time.Second
	maxFetchBytes     = 256 * 1024
	// maxDownloadBytes caps files written by the download_file tool.
	maxDownloadBytes = 20 * 1024 * 1024
)

// WebClient performs outbound web searches and page fetches. It is nil-safe
// absent: a Sandbox without one reports the web tools as disabled, keeping
// runs offline by default.
type WebClient struct {
	// client serves web_fetch / http_request / download_file. Its transport
	// enforces the address policy at dial time (see policyTransport), so the
	// IP that passes validation is the IP the connection actually uses.
	client *http.Client
	// searchClient serves the operator-configured search endpoint only. That
	// endpoint is trusted like AllowLocal — an internal SearXNG instance is
	// a supported setup — so it deliberately bypasses the address policy.
	searchClient *http.Client
	searchURL    string

	// AllowLocal permits loopback/private hosts and non-standard ports.
	// Production defaults to false; tests point at httptest servers.
	AllowLocal bool
}

// NewWebClient builds a client using searchURL as the query template
// (must contain the "{query}" placeholder); empty means the built-in
// DuckDuckGo lite endpoint. A SearXNG instance is used when its template
// contains "format=json".
func NewWebClient(searchURL string, timeout time.Duration) (*WebClient, error) {
	if strings.TrimSpace(searchURL) == "" {
		searchURL = defaultSearchURL
	}
	if !strings.Contains(searchURL, "{query}") {
		return nil, fmt.Errorf("search URL must contain a {query} placeholder: %s", searchURL)
	}
	if timeout <= 0 {
		timeout = defaultWebTimeout
	}
	w := &WebClient{searchURL: searchURL}
	w.client = &http.Client{
		Timeout: timeout,
		// Re-validate every hop; redirects must not smuggle the fetch to a
		// private address. The dial-time check in policyTransport is the
		// authoritative enforcement; this catches the hop early and clearly.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return w.validateURL(req.Context(), req.URL)
		},
		Transport: w.policyTransport(),
	}
	w.searchClient = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return nil
		},
	}
	return w, nil
}

// policyTransport builds the fetch/download transport. Its DialContext is
// the enforcement point of the address policy: the host is resolved and
// checked immediately before connecting, and the connection goes to one of
// the checked IPs, so no DNS answer can change between validation and use
// (the classic rebinding TOCTOU — validating the URL and then letting the
// transport re-resolve is not enough). No proxy is configured on purpose:
// routing through one would move the check onto the proxy's own address.
// HTTP/2 is left off because a custom DialContext without DialTLSContext
// cannot negotiate it; page fetches do not need it.
func (w *WebClient) policyTransport() *http.Transport {
	return &http.Transport{
		DialContext:           w.secureDialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// secureDialContext runs inside the transport for the initial request and
// every redirect hop: resolve addr, apply the policy, dial one of the
// validated addresses. AllowLocal skips the policy entirely.
func (w *WebClient) secureDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address %q: %w", addr, err)
	}
	dialer := &net.Dialer{Timeout: defaultWebTimeout}
	if w.AllowLocal {
		return dialer.DialContext(ctx, network, addr)
	}
	ips, err := w.resolvePublicIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// resolvePublicIPs resolves host and enforces the address policy on every
// result; IP literals are checked directly. It is the single source of
// truth shared by the up-front URL validation and the dial-time check, so
// the two can never drift apart.
func (w *WebClient) resolvePublicIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("address %s is not public", ip)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %q: %w", host, err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if !isPublicIP(a.IP) {
			return nil, fmt.Errorf("host %q resolves to a non-public address %s", host, a.IP)
		}
		ips = append(ips, a.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q has no addresses", host)
	}
	return ips, nil
}

// Search runs the query and returns compact numbered results:
// "1. <title>\n   <url>\n   <snippet>".
func (w *WebClient) Search(ctx context.Context, query string, max int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("empty query")
	}
	if max <= 0 {
		max = 5
	}

	endpoint := strings.ReplaceAll(w.searchURL, "{query}", url.QueryEscape(query))
	body, err := w.get(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}

	var results [][3]string
	if strings.Contains(w.searchURL, "format=json") {
		if results, err = parseSearXNG(body); err != nil {
			return "", err
		}
	} else {
		results = parseDDGLite(body)
	}
	if len(results) == 0 {
		return "(no results — the search endpoint may have changed or blocked the request)", nil
	}

	var b strings.Builder
	for i, r := range results {
		if i >= max {
			break
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, r[0], r[1], collapseWS(r[2]))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// Fetch downloads a page and returns readable text: HTML is converted to
// plain text, other text formats pass through.
func (w *WebClient) Fetch(ctx context.Context, rawURL string) (string, error) {
	return w.request(ctx, http.MethodGet, rawURL, nil, "", false)
}

// Do performs a structured HTTP request and returns a compact
// "status/headers/body" report. Used by the http_request tool.
func (w *WebClient) Do(ctx context.Context, method, rawURL string, headers map[string]string, body string) (string, error) {
	return w.request(ctx, method, rawURL, headers, body, true)
}

// Download streams a public http(s) URL into out with the same scheme,
// private-address, port, and redirect policy as Fetch. It requires a 2xx
// response and aborts once more than maxDownloadBytes arrive (the caller
// must discard the partial destination on error); the content type is
// returned for confirmation.
func (w *WebClient) Download(ctx context.Context, rawURL string, out io.Writer) (string, int64, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", 0, fmt.Errorf("invalid URL: %w", err)
	}
	if err := w.validateURL(ctx, u); err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "divvy/1.0 (+download)")
	resp, err := w.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("download failed: %s", resp.Status)
	}
	// One extra byte reveals an oversized body before it silently truncates.
	n, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return "", n, err
	}
	if n > maxDownloadBytes {
		return "", n, fmt.Errorf("download exceeds %d-byte limit", maxDownloadBytes)
	}
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ct, n, nil
}

// request implements both Fetch (GET, HTML→text) and Do (raw report).
func (w *WebClient) request(ctx context.Context, method, rawURL string, headers map[string]string, body string, rawReport bool) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if err := w.validateURL(ctx, u); err != nil {
		return "", err
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "divvy/1.0 (+web)")
	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", err
	}

	if rawReport {
		var b strings.Builder
		fmt.Fprintf(&b, "status: %s\n", resp.Status)
		for k := range resp.Header {
			fmt.Fprintf(&b, "header: %s: %s\n", k, strings.Join(resp.Header.Values(k), ", "))
		}
		b.WriteString("body:\n")
		b.Write(data)
		return strings.TrimSpace(b.String()), nil
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "html"):
		return strings.TrimSpace(htmlToText(string(data))), nil
	case strings.HasPrefix(ct, "text/"), strings.Contains(ct, "json"), strings.Contains(ct, "xml"):
		return strings.TrimSpace(string(data)), nil
	default:
		return "", fmt.Errorf("unsupported content type %q (only HTML/text/JSON/XML)", ct)
	}
}

// get performs a GET and returns the body. The operator-configured search
// endpoint is trusted even when it resolves locally (e.g. an internal
// SearXNG), so it uses searchClient and bypasses validateURL.
func (w *WebClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "divvy/1.0 (+web search)")
	resp, err := w.searchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4*maxFetchBytes))
}

// validateURL enforces the fetch safety policy up front: public schemes,
// standard ports, non-private destination IPs. The up-front check exists to
// fail fast with a clear message; the dial-time check in secureDialContext
// is what actually guarantees the connected address was validated.
func (w *WebClient) validateURL(ctx context.Context, u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are allowed, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL is missing a host")
	}
	port := u.Port()
	switch port {
	case "", "80", "443":
	default:
		if !w.AllowLocal {
			return fmt.Errorf("port %q is not allowed (80/443 only)", port)
		}
	}

	if w.AllowLocal {
		return nil
	}
	_, err := w.resolvePublicIPs(ctx, host)
	return err
}

// isPublicIP reports whether ip is outside loopback/private/reserved ranges.
// Go's net.IP predicates cover the RFC1918/loopback/link-local basics but
// not every non-globally-routable block, so the remainder is explicit:
// CGNAT and NAT64 targets can be internal services behind a carrier/NAT64
// gateway, and the reserved/documentation/benchmarking blocks are black
// holes at best.
func isPublicIP(ip net.IP) bool {
	switch {
	case ip.IsLoopback(), ip.IsPrivate(), ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(),
		ip.IsUnspecified(), ip.IsMulticast():
		return false
	}
	// To4 also normalizes IPv4-mapped IPv6 (::ffff:1.2.3.4), so one branch
	// covers both wire formats.
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8 "this network"
			return false
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64.0.0/10 CGNAT
			return false
		case v4[0] == 192 && v4[1] == 0 && (v4[2] == 0 || v4[2] == 2): // 192.0.0.0/24, TEST-NET-1
			return false
		case v4[0] == 192 && v4[1] == 88 && v4[2] == 99: // 192.88.99.0/24 6to4 relay anycast
			return false
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // 198.18.0.0/15 benchmarking
			return false
		case v4[0] == 198 && v4[1] == 51 && v4[2] == 100: // 198.51.100.0/24 TEST-NET-2
			return false
		case v4[0] == 203 && v4[1] == 0 && v4[2] == 113: // 203.0.113.0/24 TEST-NET-3
			return false
		case v4[0] >= 240: // 240.0.0.0/4 reserved + 255.255.255.255 broadcast
			return false
		}
		return true
	}
	if len(ip) == net.IPv6len {
		switch {
		case ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b: // 64:ff9b::/96 NAT64
			return false
		case ip[0] == 0xfe && ip[1] == 0xc0: // fec0::/10 site-local (deprecated)
			return false
		case ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8: // 2001:db8::/32
			return false
		}
	}
	return true
}

var (
	// ddgLink matches result anchors regardless of attribute order; the
	// href is a redirect whose uddg query parameter holds the real URL.
	ddgLink = regexp.MustCompile(`(?is)<a([^>]*?)class="result-link"[^>]*>(.*?)</a>`)
	ddgHref = regexp.MustCompile(`(?is)href="([^"]+)"`)
	ddgSnp  = regexp.MustCompile(`(?is)<td[^>]+class="result-snippet"[^>]*>(.*?)</td>`)
	// RE2 has no backreferences, so script/style blocks are stripped by two
	// separate expressions.
	scriptTagRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
)

// stripBlockTags removes script and style element contents.
func stripBlockTags(s string) string {
	s = scriptTagRe.ReplaceAllString(s, " ")
	return styleTagRe.ReplaceAllString(s, " ")
}

// parseDDGLite extracts title/url/snippet triples from the lite HTML page.
func parseDDGLite(body []byte) [][3]string {
	s := string(body)
	links := ddgLink.FindAllStringSubmatch(s, -1)
	snips := ddgSnp.FindAllStringSubmatch(s, -1)

	out := make([][3]string, 0, len(links))
	for i, m := range links {
		href := ""
		if hm := ddgHref.FindStringSubmatch(m[1]); hm != nil {
			href = html.UnescapeString(hm[1])
		}
		title := collapseWS(htmlToText(m[2]))
		target := href
		if ru, err := url.Parse(strings.TrimPrefix(href, "//")); err == nil {
			if uddg := ru.Query().Get("uddg"); uddg != "" {
				target = uddg
			}
		}
		if strings.HasPrefix(target, "//") {
			target = "https:" + target
		}
		snippet := ""
		if i < len(snips) {
			snippet = htmlToText(snips[i][1])
		}
		out = append(out, [3]string{title, target, snippet})
	}
	return out
}

type searXNGResponse struct {
	Results []struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"results"`
}

// parseSearXNG parses the JSON response of a SearXNG instance
// ("?format=json").
func parseSearXNG(body []byte) ([][3]string, error) {
	var r searXNGResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("invalid SearXNG JSON: %w", err)
	}
	out := make([][3]string, 0, len(r.Results))
	for _, item := range r.Results {
		out = append(out, [3]string{item.Title, item.URL, item.Content})
	}
	return out, nil
}

// htmlToText strips scripts and tags from HTML and unescapes entities.
func htmlToText(s string) string {
	s = stripBlockTags(s)
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			b.WriteByte(' ')
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return collapseWS(html.UnescapeString(b.String()))
}

func collapseWS(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
