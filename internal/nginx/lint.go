package nginx

import (
	"fmt"
	"net/url"
	"strings"
)

// A small nginx config linter for common, well-known pitfalls.
// It parses text into a directive tree (no includes resolved) and runs rules
// over it. Findings are advisory; `nginx -t` remains the source of truth.

type Directive struct {
	Name     string
	Args     []string
	Line     int
	Children []*Directive
	Block    bool
}

type LintFinding struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"` // error | warning | info
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Doc      string `json:"doc,omitempty"`
}

// parseDirectives tokenizes nginx syntax: words, quoted strings, ; { } and # comments.
func ParseDirectives(src string) []*Directive {
	type tok struct {
		s    string
		line int
	}
	var toks []tok
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == ';' || c == '{' || c == '}':
			toks = append(toks, tok{string(c), line})
			i++
		case c == '"' || c == '\'':
			q, start, startLine := c, i+1, line
			i++
			for i < len(src) && src[i] != q {
				if src[i] == '\\' {
					i++
				} else if src[i] == '\n' {
					line++
				}
				i++
			}
			end := i
			if end > len(src) {
				end = len(src)
			}
			toks = append(toks, tok{src[start:end], startLine})
			i++
		default:
			start := i
			for i < len(src) && !strings.ContainsRune(" \t\r\n;{}#", rune(src[i])) {
				i++
			}
			toks = append(toks, tok{src[start:i], line})
		}
	}

	root := &Directive{Block: true}
	stack := []*Directive{root}
	var cur *Directive
	for _, t := range toks {
		parent := stack[len(stack)-1]
		switch t.s {
		case ";":
			if cur != nil {
				parent.Children = append(parent.Children, cur)
				cur = nil
			}
		case "{":
			if cur == nil {
				cur = &Directive{Line: t.line}
			}
			cur.Block = true
			parent.Children = append(parent.Children, cur)
			stack = append(stack, cur)
			cur = nil
		case "}":
			if cur != nil { // directive missing ';' before '}' — nginx -t will report it
				parent.Children = append(parent.Children, cur)
				cur = nil
			}
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		default:
			if cur == nil {
				cur = &Directive{Name: t.s, Line: t.line}
			} else {
				cur.Args = append(cur.Args, t.s)
			}
		}
	}
	return root.Children
}

func (d *Directive) child(name string) *Directive {
	for _, c := range d.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (d *Directive) has(name string) bool { return d.child(name) != nil }

func Lint(src string) []LintFinding {
	findings := []LintFinding{}
	add := func(line int, sev, rule, msg, doc string) {
		findings = append(findings, LintFinding{Line: line, Severity: sev, Rule: rule, Message: msg, Doc: doc})
	}

	var walk func(nodes []*Directive, ancestors []*Directive)
	walk = func(nodes []*Directive, ancestors []*Directive) {
		for _, d := range nodes {
			switch d.Name {
			case "ssl_protocols":
				for _, a := range d.Args {
					if a == "SSLv3" || a == "TLSv1" || a == "TLSv1.1" {
						add(d.Line, "error", "weak-tls",
							fmt.Sprintf("%s is deprecated and insecure. Use `ssl_protocols TLSv1.2 TLSv1.3;`", a),
							"https://nginx.org/en/docs/http/ngx_http_ssl_module.html#ssl_protocols")
					}
				}
			case "server_tokens":
				if len(d.Args) > 0 && d.Args[0] == "on" {
					add(d.Line, "info", "server-tokens", "`server_tokens on` reveals the nginx version in headers and error pages. Consider `server_tokens off;`",
						"https://nginx.org/en/docs/http/ngx_http_core_module.html#server_tokens")
				}
			case "autoindex":
				if len(d.Args) > 0 && d.Args[0] == "on" {
					add(d.Line, "warning", "autoindex", "`autoindex on` publicly lists directory contents. Make sure this is intended.",
						"https://nginx.org/en/docs/http/ngx_http_autoindex_module.html")
				}
			case "location":
				lintLocation(d, ancestors, add)
			case "if":
				if inLocation(ancestors) {
					for _, c := range d.Children {
						if c.Name != "return" && c.Name != "rewrite" && c.Name != "set" {
							add(d.Line, "warning", "if-in-location",
								fmt.Sprintf("`%s` inside `if` within a location can behave unexpectedly (\"if is evil\"). Only `return`/`rewrite` are fully safe; prefer `map` or separate locations.", c.Name),
								"https://www.nginx.com/resources/wiki/start/topics/depth/ifisevil/")
							break
						}
					}
				}
			}

			// add_header in a child block discards ALL add_header from parents
			if d.Block && d.has("add_header") {
				for i := len(ancestors) - 1; i >= 0; i-- {
					if ancestors[i].has("add_header") {
						add(d.child("add_header").Line, "warning", "add-header-inheritance",
							fmt.Sprintf("`add_header` here stops inheritance: headers set in the enclosing `%s` block (line %d), e.g. security headers, are no longer sent from this block.", ancestors[i].Name, ancestors[i].child("add_header").Line),
							"https://nginx.org/en/docs/http/ngx_http_headers_module.html#add_header")
						break
					}
				}
			}

			if d.Block {
				walk(d.Children, append(append([]*Directive{}, ancestors...), d))
			}
		}
	}
	walk(ParseDirectives(src), nil)
	return findings
}

func inLocation(ancestors []*Directive) bool {
	for _, a := range ancestors {
		if a.Name == "location" {
			return true
		}
	}
	return false
}

func lintLocation(loc *Directive, ancestors []*Directive, add func(int, string, string, string, string)) {
	pp := loc.child("proxy_pass")
	if pp == nil || len(pp.Args) == 0 {
		return
	}
	// Prefix location (not regex / named): check trailing-slash mapping.
	if len(loc.Args) == 1 || (len(loc.Args) == 2 && loc.Args[0] == "^~") {
		prefix := loc.Args[len(loc.Args)-1]
		if u, err := url.Parse(pp.Args[0]); err == nil && u.Host != "" && !strings.Contains(pp.Args[0], "$") && u.Path != "" {
			switch {
			case !strings.HasSuffix(prefix, "/") && strings.HasSuffix(u.Path, "/"):
				add(pp.Line, "warning", "proxy-pass-slash",
					fmt.Sprintf("`location %s` + `proxy_pass …%s` turns %s/foo into %sfoo with a double slash (//foo). Use `location %s/` or drop the trailing slash.", prefix, u.Path, prefix, u.Path, prefix),
					"https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass")
			case strings.HasSuffix(prefix, "/") && !strings.HasSuffix(u.Path, "/"):
				add(pp.Line, "warning", "proxy-pass-slash",
					fmt.Sprintf("`location %s` + `proxy_pass …%s` joins paths without a slash: %sfoo → %sfoo. Add a trailing slash to the proxy_pass URI.", prefix, u.Path, prefix, u.Path),
					"https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass")
			}
		}
	}
	// Host header
	setsHeaders, setsHost := false, false
	for _, c := range loc.Children {
		if c.Name == "proxy_set_header" {
			setsHeaders = true
			if len(c.Args) > 0 && strings.EqualFold(c.Args[0], "Host") {
				setsHost = true
			}
		}
	}
	if setsHeaders && !setsHost {
		add(pp.Line, "warning", "proxy-host-header",
			"This location sets `proxy_set_header` but not `Host`. Any headers set in parent blocks (including Host) are NOT inherited, so the upstream receives `$proxy_host`. Add `proxy_set_header Host $host;`",
			"https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_set_header")
	}
	if !setsHeaders {
		inherited := false
		for _, a := range ancestors {
			if a.has("proxy_set_header") {
				inherited = true
			}
		}
		if !inherited {
			add(pp.Line, "info", "proxy-host-header",
				"No `proxy_set_header Host` here or in parent blocks: the upstream sees the upstream name as Host, not the client's. Usually you want `proxy_set_header Host $host;`",
				"https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_set_header")
		}
	}
}
