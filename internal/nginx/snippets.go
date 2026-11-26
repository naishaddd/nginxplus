package nginx

import "strings"

// Snippet template generator: keyword-matched templates (no LLM yet).
func SnippetTemplate(prompt string) (string, string) {
	lower := strings.ToLower(prompt)
	var explanation string
	var snippet string

	switch {
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "ratelimit") || strings.Contains(lower, "limit req"):
		explanation = "Configured binary remote address rate limiting zone with burst tolerance and nodelay."
		snippet = `# Rate limiting zone definition (place in http block or conf.d file)
limit_req_zone $binary_remote_addr zone=ip_limit:10m rate=10r/s;

# Location block applying limit
location / {
    limit_req zone=ip_limit burst=20 nodelay;
    limit_req_status 429;
}
`
	case strings.Contains(lower, "https") || strings.Contains(lower, "ssl redirect") || strings.Contains(lower, "redirect to https"):
		explanation = "HTTP 301 Permanent Redirect to HTTPS preserving URI query parameters and request host."
		snippet = `server {
    listen 80;
    server_name example.com *.example.com;
    return 301 https://$host$request_uri;
}
`
	case strings.Contains(lower, "cache") || strings.Contains(lower, "static") || strings.Contains(lower, "expires"):
		explanation = "High-performance asset caching with aggressive 30-day Cache-Control header and access logging disabled."
		snippet = `location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2|svg)$ {
    expires 30d;
    add_header Cache-Control "public, no-transform";
    access_log off;
    log_not_found off;
}
`
	case strings.Contains(lower, "cors"):
		explanation = "Full CORS preflight headers allowing cross-origin GET, POST, OPTIONS requests."
		snippet = `if ($request_method = 'OPTIONS') {
    add_header 'Access-Control-Allow-Origin' '*';
    add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS, PUT, DELETE';
    add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization';
    add_header 'Access-Control-Max-Age' 1728000;
    add_header 'Content-Type' 'text/plain; charset=utf-8';
    add_header 'Content-Length' 0;
    return 204;
}
add_header 'Access-Control-Allow-Origin' '*' always;
`
	default:
		explanation = "Reverse proxy configuration template with WebSocket support and forwarded IP headers."
		snippet = `location /api/ {
    proxy_pass http://api_cluster;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_cache_bypass $http_upgrade;
}
`
	}

	return explanation, snippet
}

// httpLevelDirectives must live in the http{} context, not inside server{}.
var httpLevelDirectives = []string{"limit_req_zone", "limit_conn_zone", "proxy_cache_path", "map ", "upstream ", "geo ", "split_clients "}

// splitSnippet separates http-level lines from server-level lines (best effort,
// line based; block-starting http directives keep their whole block).
func SplitSnippet(snippet string) (httpPart, serverPart string) {
	if strings.Contains(snippet, "server {") || strings.Contains(snippet, "server{") {
		return snippet, ""
	}
	var h, s []string
	depth, inHTTP := 0, false
	for _, line := range strings.Split(snippet, "\n") {
		t := strings.TrimSpace(line)
		if depth == 0 {
			inHTTP = false
			for _, d := range httpLevelDirectives {
				if strings.HasPrefix(t, d) {
					inHTTP = true
				}
			}
		}
		depth += strings.Count(t, "{") - strings.Count(t, "}")
		if inHTTP {
			h = append(h, line)
		} else {
			s = append(s, line)
		}
	}
	return strings.Join(h, "\n"), strings.Join(s, "\n")
}
