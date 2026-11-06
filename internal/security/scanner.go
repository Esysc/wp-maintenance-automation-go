package security

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Finding struct {
	ID             string   `json:"id"`
	Severity       Severity `json:"severity"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Recommendation string   `json:"recommendation"`
	Action         string   `json:"action,omitempty"`
	Evidence       string   `json:"evidence,omitempty"`
	URL            string   `json:"url,omitempty"`
	Reference      string   `json:"reference,omitempty"`
}

type Report struct {
	Target    string         `json:"target"`
	ScannedAt time.Time      `json:"scanned_at"`
	Findings  []Finding      `json:"findings"`
	Counts    map[string]int `json:"counts"`
}

type Options struct {
	Timeout            time.Duration
	MaxBody            int64
	UserAgent          string
	InsecureSkipVerify bool
}

// Scan performs a non-invasive security configuration review against a
// WordPress site. It only uses GET/HEAD/OPTIONS-style requests and never
// attempts authentication bypass, exploitation, or brute force.
func Scan(ctx context.Context, target string, opts Options) (*Report, error) {
	base, err := url.Parse(strings.TrimSpace(target))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid scan target")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 8 * time.Second
	}
	if opts.MaxBody <= 0 {
		opts.MaxBody = 1 << 20
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "WP-Maintenance-Security-Review/1.0"
	}

	s := &scanner{
		base:               base,
		ctx:                ctx,
		userAgent:          opts.UserAgent,
		maxBody:            opts.MaxBody,
		insecureSkipVerify: opts.InsecureSkipVerify,
		client: &http.Client{
			Timeout: opts.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerify},
			},
		},
	}

	report := &Report{Target: target, ScannedAt: time.Now().UTC(), Counts: map[string]int{}}
	home, err := s.get("/")
	if err != nil {
		return nil, fmt.Errorf("homepage request failed: %w", err)
	}

	s.checkTransport(report)
	s.checkHeaders(report, home)
	s.checkCookies(report)
	s.checkHomeContent(report, home)
	s.checkSensitiveFiles(report)
	s.checkWordPressEndpoints(report)
	s.checkMethods(report)

	sort.SliceStable(report.Findings, func(i, j int) bool {
		return severityRank(report.Findings[i].Severity) < severityRank(report.Findings[j].Severity)
	})
	report.applyRemediationActions()
	report.Recount()
	return report, nil
}

type scanner struct {
	base               *url.URL
	ctx                context.Context
	userAgent          string
	maxBody            int64
	insecureSkipVerify bool
	client             *http.Client
}

type probe struct {
	status   int
	header   http.Header
	body     string
	finalURL string
}

func (s *scanner) urlFor(path string) string {
	ref, err := url.Parse(path)
	if err != nil {
		return s.base.String()
	}
	return s.base.ResolveReference(ref).String()
}

func (s *scanner) get(path string) (*probe, error) {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.urlFor(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, s.maxBody))
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return &probe{status: resp.StatusCode, header: resp.Header, body: string(body), finalURL: finalURL}, nil
}

func (s *scanner) checkTransport(report *Report) {
	if !strings.EqualFold(s.base.Scheme, "https") {
		report.add("https", SeverityHigh, "Site is not served over HTTPS",
			"The scanned URL uses HTTP, so credentials and content can be intercepted.",
			"Serve the site over HTTPS and redirect all HTTP requests to HTTPS.")
		return
	}

	host := s.base.Hostname()
	port := s.base.Port()
	if port == "" {
		port = "443"
	}
	address := net.JoinHostPort(host, port)

	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: s.insecureSkipVerify,
	})
	if err != nil {
		if !s.insecureSkipVerify {
			report.add("tls-certificate", SeverityHigh, "TLS certificate could not be verified",
				fmt.Sprintf("The TLS handshake with %s failed: %v", address, err),
				"Install a valid certificate that matches the site hostname and includes the full intermediate chain.")
		}
		return
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if state.Version < tls.VersionTLS12 {
		report.add("tls-version", SeverityHigh, "Legacy TLS protocol enabled",
			fmt.Sprintf("The server negotiated %s.", tlsVersionName(state.Version)),
			"Disable TLS 1.0/1.1 and allow only TLS 1.2 or newer.")
	}
}

func (s *scanner) checkHeaders(report *Report, home *probe) {
	headers := home.header
	https := strings.EqualFold(s.base.Scheme, "https")

	if https && headers.Get("Strict-Transport-Security") == "" {
		report.add("hsts", SeverityMedium, "Missing Strict-Transport-Security header",
			"The site does not instruct browsers to use HTTPS for future visits.",
			"Send Strict-Transport-Security: max-age=31536000; includeSubDomains after confirming all subdomains support HTTPS.")
	}
	csp := headers.Get("Content-Security-Policy")
	if csp == "" {
		report.add("csp", SeverityMedium, "Missing Content-Security-Policy",
			"Without a CSP, injected scripts have fewer browser-level restrictions.",
			"Add a restrictive Content-Security-Policy, starting with default-src 'self' and frame-ancestors 'self'.")
	}
	if !strings.EqualFold(strings.TrimSpace(headers.Get("X-Content-Type-Options")), "nosniff") {
		report.add("x-content-type-options", SeverityLow, "Missing X-Content-Type-Options",
			"Browsers may MIME-sniff responses into executable content.",
			"Send X-Content-Type-Options: nosniff on all responses.")
	}
	if headers.Get("X-Frame-Options") == "" && !strings.Contains(strings.ToLower(csp), "frame-ancestors") {
		report.add("frame-options", SeverityLow, "Clickjacking protection is not configured",
			"The page can be framed by other sites.",
			"Send X-Frame-Options: SAMEORIGIN or add a CSP frame-ancestors directive.")
	}
	if headers.Get("Referrer-Policy") == "" {
		report.add("referrer-policy", SeverityLow, "Missing Referrer-Policy",
			"Referrers may leak sensitive URL paths to third parties.",
			"Send Referrer-Policy: strict-origin-when-cross-origin or a stricter policy.")
	}
	if headers.Get("Permissions-Policy") == "" {
		report.add("permissions-policy", SeverityInfo, "No Permissions-Policy header",
			"Browser features such as camera, microphone, and geolocation are not explicitly restricted.",
			"Consider a Permissions-Policy that disables unused browser features.")
	}
	if headers.Get("Server") != "" {
		report.add("server-banner", SeverityLow, "Server banner exposes software details",
			"The Server header reveals the web server product or version.",
			"Hide or minimize the Server header at the reverse proxy or web server.")
	}
	if headers.Get("X-Powered-By") != "" {
		report.add("powered-by", SeverityLow, "X-Powered-By header exposes PHP details",
			"The X-Powered-By header reveals runtime information.",
			"Disable expose_php or remove the X-Powered-By header.")
	}
	if strings.Contains(strings.ToLower(headers.Get("Access-Control-Allow-Origin")), "*") &&
		strings.EqualFold(headers.Get("Access-Control-Allow-Credentials"), "true") {
		report.add("cors", SeverityHigh, "Permissive CORS with credentials",
			"A wildcard origin combined with credentials allows any site to read authenticated responses.",
			"Replace the wildcard origin with an explicit allowlist and avoid credentials on public endpoints.")
	}
}

func (s *scanner) checkCookies(report *Report) {
	login, err := s.get("/wp-login.php")
	if err != nil || login.status != http.StatusOK {
		return
	}
	for _, raw := range login.header.Values("Set-Cookie") {
		lower := strings.ToLower(raw)
		name := strings.ToLower(strings.SplitN(raw, "=", 2)[0])
		if !strings.HasPrefix(name, "wordpress_logged_in") && !strings.HasPrefix(name, "wordpress_sec") {
			continue
		}
		if strings.EqualFold(s.base.Scheme, "https") && !strings.Contains(lower, "; secure") {
			report.add("cookie-secure", SeverityMedium, "Authentication cookie without Secure flag",
				"An authentication cookie can be sent over an unencrypted connection.",
				"Set the Secure flag on all WordPress authentication cookies.")
		}
		if !strings.Contains(lower, "; httponly") {
			report.add("cookie-httponly", SeverityMedium, "Authentication cookie without HttpOnly flag",
				"Client-side scripts can read the authentication cookie.",
				"Set the HttpOnly flag on all WordPress authentication cookies.")
		}
	}
}

func (s *scanner) checkHomeContent(report *Report, home *probe) {
	if m := generatorRe.FindStringSubmatch(home.body); len(m) == 2 {
		report.add("generator", SeverityLow, "Generator meta tag discloses software details",
			fmt.Sprintf("The page generator meta tag exposes %q.", m[1]),
			"Remove generator meta tags added by WordPress, themes, or plugins.")
	}
	if debugOutputRe.MatchString(home.body) {
		report.add("debug-output", SeverityHigh, "PHP errors are displayed in the page",
			"Debug output can reveal filesystem paths, database details, and plugin internals.",
			"Set WP_DEBUG_DISPLAY to false, enable WP_DEBUG_LOG, and fix the underlying warnings.")
	}
	if strings.EqualFold(s.base.Scheme, "https") && mixedContentRe.MatchString(home.body) {
		report.add("mixed-content", SeverityMedium, "Mixed HTTP content on an HTTPS page",
			"The page references resources over HTTP, weakening transport security.",
			"Load all scripts, styles, images, and fonts over HTTPS.")
	}
}

func (s *scanner) checkSensitiveFiles(report *Report) {
	for _, candidate := range []struct {
		id        string
		path      string
		severity  Severity
		title     string
		recommend string
		match     func(string) bool
	}{
		{"wp-config-backup", "/wp-config.php.bak", SeverityCritical, "Exposed wp-config.php backup",
			"Remove backup copies of wp-config.php from the web root and rotate database credentials.",
			func(body string) bool { return containsAllFold(body, "DB_NAME", "DB_PASSWORD") }},
		{"wp-config-backup", "/wp-config.php~", SeverityCritical, "Exposed wp-config.php backup",
			"Remove backup copies of wp-config.php from the web root and rotate database credentials.",
			func(body string) bool { return containsAllFold(body, "DB_NAME", "DB_PASSWORD") }},
		{"wp-config-backup", "/wp-config.php.old", SeverityCritical, "Exposed wp-config.php backup",
			"Remove backup copies of wp-config.php from the web root and rotate database credentials.",
			func(body string) bool { return containsAllFold(body, "DB_NAME", "DB_PASSWORD") }},
		{"env-file", "/.env", SeverityHigh, "Exposed .env file",
			"Remove environment files from the web root and rotate any exposed secrets.",
			func(body string) bool {
				return envLineRe.MatchString(body) && !strings.Contains(strings.ToLower(body), "<html")
			}},
		{"git-config", "/.git/config", SeverityCritical, "Exposed Git repository metadata",
			"Remove .git from the web root and redeploy without repository metadata.",
			func(body string) bool { return strings.Contains(body, "[core]") }},
		{"debug-log", "/wp-content/debug.log", SeverityHigh, "Exposed WordPress debug log",
			"Delete or move wp-content/debug.log and block direct access to it.",
			func(body string) bool {
				return strings.Contains(body, "PHP") || strings.Contains(body, "Stack trace") || strings.Contains(body, "WordPress database error")
			}},
	} {
		if s.probeOK(candidate.path, candidate.match) {
			report.add(candidate.id, candidate.severity, candidate.title,
				fmt.Sprintf("%s is publicly readable.", candidate.path), candidate.recommend)
		}
	}

	for _, dir := range []string{"/wp-content/uploads/", "/wp-content/plugins/", "/wp-content/themes/"} {
		if s.probeOK(dir, func(body string) bool {
			return strings.Contains(body, "Index of /") || strings.Contains(body, "Index of")
		}) {
			report.add("directory-listing", SeverityMedium, "Directory listing is enabled",
				fmt.Sprintf("%s exposes a browsable directory listing.", dir),
				"Disable directory listing with Options -Indexes and verify upload directories only serve intended files.")
		}
	}

	if s.probeOK("/readme.html", func(body string) bool { return strings.Contains(body, "WordPress") }) {
		report.add("readme", SeverityLow, "WordPress readme.html is publicly accessible",
			"The readme file can disclose the WordPress version.",
			"Remove readme.html and license.txt from the web root.")
	}
	if s.probeOK("/license.txt", func(body string) bool { return strings.Contains(body, "WordPress") }) {
		report.add("license", SeverityInfo, "WordPress license.txt is publicly accessible",
			"The license file is not sensitive but is often used for fingerprinting.",
			"Remove unnecessary readme.html and license.txt files from the web root.")
	}
	if !s.probeOK("/.well-known/security.txt", func(body string) bool { return strings.Contains(strings.ToLower(body), "contact:") }) {
		report.add("security-txt", SeverityInfo, "No security.txt contact",
			"Security researchers have no published contact path.",
			"Publish /.well-known/security.txt with a security contact and disclosure policy.")
	}
}

func (s *scanner) checkWordPressEndpoints(report *Report) {
	if p, err := s.get("/xmlrpc.php"); err == nil &&
		(p.status == http.StatusOK || p.status == http.StatusMethodNotAllowed) &&
		(strings.Contains(p.body, "XML-RPC server accepts POST requests only") || strings.Contains(strings.ToLower(p.body), "xml-rpc")) {
		report.add("xmlrpc", SeverityMedium, "XML-RPC endpoint is enabled",
			"XML-RPC can be used for brute-force amplification and pingback abuse.",
			"Disable XML-RPC if it is not needed, or restrict it to allowlisted IP addresses.")
	}

	if s.probeOK("/wp-json/wp/v2/users", func(body string) bool {
		return strings.Contains(body, "\"slug\"") && strings.Contains(body, "\"name\"")
	}) {
		report.add("user-enumeration", SeverityMedium, "REST API exposes user accounts",
			"/wp-json/wp/v2/users can disclose usernames used for login attacks.",
			"Require authentication for the users endpoint or restrict it at the web server.")
	}

	if p, err := s.get("/wp-admin/install.php"); err == nil && p.status == http.StatusOK &&
		!strings.Contains(p.body, "Already Installed") {
		report.add("installer", SeverityHigh, "WordPress installer is reachable",
			"/wp-admin/install.php is accessible and may allow reinstallation or reconfiguration.",
			"Block /wp-admin/install.php after installation.")
	}

	if p, err := s.get("/wp-login.php"); err == nil && p.status == http.StatusOK {
		report.add("login", SeverityInfo, "WordPress login page is reachable",
			"The login page is expected, but it is a target for credential attacks.",
			"Enable two-factor authentication and login rate limiting for all administrator accounts.")
	}

	if p, err := s.get("/wp-json/"); err == nil && p.status == http.StatusOK {
		report.add("rest-api", SeverityInfo, "WordPress REST API is publicly reachable",
			"The REST API is expected for many plugins and blocks, but its surface should be reviewed.",
			"Review REST routes and restrict any that expose sensitive data.")
	}
}

func (s *scanner) checkMethods(report *Report) {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodTrace, s.base.String(), nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		report.add("trace", SeverityMedium, "HTTP TRACE method is enabled",
			"The TRACE method can reflect request headers and aid cross-site tracing attacks.",
			"Disable TRACE and TRACK in the web server configuration.")
	}
}

func (s *scanner) probeOK(path string, match func(string) bool) bool {
	p, err := s.get(path)
	if err != nil || p.status != http.StatusOK {
		return false
	}
	return match(p.body)
}

func (r *Report) add(id string, severity Severity, title, description, recommendation string) {
	r.Findings = append(r.Findings, Finding{
		ID:             id,
		Severity:       severity,
		Title:          title,
		Description:    description,
		Recommendation: recommendation,
	})
}

// AddFindings appends findings and refreshes remediation steps, ordering, and counts.
func (r *Report) AddFindings(findings ...Finding) {
	r.Findings = append(r.Findings, findings...)
	r.applyRemediationActions()
	r.Recount()
}

// Recount re-sorts findings by severity and recomputes the counts map.
func (r *Report) Recount() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		return severityRank(r.Findings[i].Severity) < severityRank(r.Findings[j].Severity)
	})
	r.Counts = map[string]int{}
	for _, finding := range r.Findings {
		r.Counts[string(finding.Severity)]++
	}
}

func (r *Report) Summary() string {
	if len(r.Findings) == 0 {
		return "no security findings"
	}
	parts := make([]string, 0, 4)
	for _, severity := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		if count := r.Counts[string(severity)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, severity))
		}
	}
	return strings.Join(parts, ", ")
}

func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 3
	default:
		return 4
	}
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04x", version)
	}
}

func containsAllFold(body string, values ...string) bool {
	lower := strings.ToLower(body)
	for _, value := range values {
		if !strings.Contains(lower, strings.ToLower(value)) {
			return false
		}
	}
	return true
}

var (
	generatorRe    = regexp.MustCompile(`(?is)<meta[^>]+name=["']generator["'][^>]+content=["']([^"']+)`)
	debugOutputRe  = regexp.MustCompile(`(?i)\b(?:Warning|Fatal error|Notice|Deprecated|Parse error):`)
	mixedContentRe = regexp.MustCompile(`(?i)(?:src|href)=["']http://|url\(["']?http://`)
	envLineRe      = regexp.MustCompile(`(?m)^[A-Z0-9_]{2,}=[^\s].*$`)
)
