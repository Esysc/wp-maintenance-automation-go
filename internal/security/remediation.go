package security

import "strings"

// applyRemediationActions attaches concrete fix steps to findings that do not
// already carry provider-specific instructions.
func (r *Report) applyRemediationActions() {
	for i := range r.Findings {
		if strings.TrimSpace(r.Findings[i].Action) != "" {
			continue
		}
		r.Findings[i].Action = remediationFor(r.Findings[i].ID)
	}
}

func remediationFor(id string) string {
	if action, ok := remediationActions[id]; ok {
		return action
	}
	switch {
	case strings.HasPrefix(id, "wpscan-"):
		return `Update the affected WordPress component to the fixed version.
Run the update in staging first, then production:
  wp core update
  wp plugin update <slug>
  wp theme update <slug>
If no fixed version is available, remove or replace the component.`
	case strings.HasPrefix(id, "osv-"):
		return `Update the affected Composer dependency:
  composer update <vendor/package>
If the dependency is bundled inside a plugin or theme, update that parent
component and redeploy it.`
	}
	return ""
}

var remediationActions = map[string]string{
	"https": `1. Install a valid TLS certificate for the site.
2. Redirect all HTTP requests to HTTPS in the web server or reverse proxy.
3. Update WordPress siteurl/home to https:// and replace remaining http:// URLs
   in the database and theme/plugin files.`,

	"hsts": `Add the header at the reverse proxy or web server after confirming all
subdomains support HTTPS:
  Apache: Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
  Nginx:  add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;`,

	"csp": `Start with a report-only policy, then enforce a restrictive policy:
  Content-Security-Policy: default-src 'self'; frame-ancestors 'self'
Allow only the CDNs, fonts, and inline scripts the site actually needs.`,

	"x-content-type-options": `Add this header to all responses at the web server or reverse proxy:
  X-Content-Type-Options: nosniff`,

	"frame-options": `Add clickjacking protection at the web server or reverse proxy:
  X-Frame-Options: SAMEORIGIN
or use a CSP frame-ancestors directive.`,

	"referrer-policy": `Add this header at the web server or reverse proxy:
  Referrer-Policy: strict-origin-when-cross-origin`,

	"permissions-policy": `Add a Permissions-Policy that enables only the features the site uses:
  Permissions-Policy: camera=(), microphone=(), geolocation=()`,

	"server-banner": `Reduce software disclosure:
  Apache: ServerTokens Prod
  Apache: ServerSignature Off
  Nginx:  server_tokens off;
Also normalize the Server header at the CDN or reverse proxy.`,

	"powered-by": `Set expose_php = Off in php.ini and restart PHP/Apache.
Remove X-Powered-By at the reverse proxy if it is still present.`,

	"cors": `Replace Access-Control-Allow-Origin: * with an explicit allowlist.
Do not combine wildcard origins with Access-Control-Allow-Credentials: true.`,

	"generator": `Remove generator tags in a child theme or mu-plugin:
  remove_action('wp_head', 'wp_generator');
Also disable version/generator output in Elementor and other plugins where possible.`,

	"debug-output": `Add these lines to wp-config.php:
  define('WP_DEBUG_DISPLAY', false);
  define('WP_DEBUG_LOG', true);
Then fix the warnings/notices recorded in wp-content/debug.log.`,

	"mixed-content": `Replace http:// asset URLs with https://.
Example:
  wp search-replace 'http://example.com' 'https://example.com' --all-tables
Then clear page caches and any page-builder CSS/JS caches.`,

	"wp-config-backup": `1. Delete the backup file from the web root.
2. Rotate the database password and update wp-config.php.
3. Block backup/config patterns (*.bak, *.old, *~, wp-config.php.*) at the web server.`,

	"env-file": `1. Remove the .env file from the web root.
2. Rotate every secret it contained.
3. Block dotfiles and .env* at the web server.`,

	"git-config": `1. Remove .git from the web root.
2. Redeploy from a clean build artifact instead of a working copy.
3. Rotate any credentials or salts that may have been exposed.`,

	"debug-log": `1. Delete wp-content/debug.log.
2. Block direct access to it, or write debug logs outside the web root.
3. Review the log for fatal errors and fix the underlying causes.`,

	"directory-listing": `Disable directory listing:
  Apache: Options -Indexes
  Nginx:  autoindex off;
Add an empty index.php or index.html to upload/content directories.`,

	"readme": `Delete readme.html and license.txt from the web root.
Repeat after WordPress updates, or block direct access to them.`,

	"license": `Delete license.txt and readme.html from the web root.
These files are not needed for the site to run.`,

	"security-txt": `Create /.well-known/security.txt with a security contact, for example:
  Contact: mailto:security@example.com
  Expires: 2027-12-31T23:59:00.000Z
  Preferred-Languages: en
  Canonical: https://example.com/.well-known/security.txt`,

	"xmlrpc": `If XML-RPC is not needed, disable it:
  add_filter('xmlrpc_enabled', '__return_false');
Alternatively block /xmlrpc.php at the reverse proxy or WAF.`,

	"user-enumeration": `Block anonymous access to /wp-json/wp/v2/users at the web server or WAF.
If a plugin needs that route, require authentication for it.`,

	"installer": `Block /wp-admin/install.php at the web server, for example:
  Apache: <Files "install.php"> Require all denied </Files>
  Nginx:  location = /wp-admin/install.php { deny all; }`,

	"login": `1. Enable two-factor authentication for every administrator.
2. Add login rate limiting and a WAF rule for /wp-login.php.
3. Remove unused administrator accounts.`,

	"rest-api": `Review the REST routes exposed by plugins and themes.
Require authentication or block routes that expose user, order, or configuration data.
Keep the endpoints required by the block editor available.`,

	"trace": `Disable the TRACE method:
  Apache: TraceEnable Off
  Nginx:  if ($request_method = TRACE) { return 405; }`,

	"tls-version": `Disable TLS 1.0 and TLS 1.1.
Allow only TLS 1.2 and TLS 1.3 in the web server or reverse proxy.`,

	"php-eol": `Upgrade PHP to a supported release (8.3 or newer at the time of writing).
Update PHP extensions, then re-run the site healthcheck and rehearsal.`,

	"cve-lookup-not-configured": `Create a WPScan API token at https://wpscan.com/api.
Set WPSCAN_API_TOKEN for the API container and restart it.`,

	"cve-lookup-incomplete": `Wait for the WPScan quota to reset, or raise WPSCAN_MAX_REQUESTS on a paid plan.
The listed components were not checked for CVEs in this run.`,

	"cve-lookup-wpscan-failed": `Verify outbound HTTPS access from the API container.
Check proxy, firewall, and DNS settings, then re-run the scan.`,

	"cve-lookup-osv-failed": `Verify outbound HTTPS access from the API container to api.osv.dev.
Check proxy, firewall, and DNS settings, then re-run the scan.`,

	"cve-lookup-php-eol-failed": `Verify outbound HTTPS access from the API container to endoflife.date.
Check proxy, firewall, and DNS settings, then re-run the scan.`,

	"osv-advisories-truncated": `Review the OSV package page and update the dependency to remove the remaining advisories.`,

	"inventory-warning": `Verify WP-CLI is installed and available to the SSH user:
  ssh user@host 'wp --info'
Install WP-CLI or add it to the non-interactive PATH, then re-run the scan.`,
}
