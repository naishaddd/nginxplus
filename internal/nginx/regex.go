package nginx

import "regexp"

// compileNginxRegex compiles an nginx (PCRE) regex with Go's RE2. Most location
// regexes are compatible; lookarounds and backreferences are not.
func compileNginxRegex(expr string, caseInsensitive bool) (*regexp.Regexp, error) {
	if caseInsensitive {
		expr = "(?i)" + expr
	}
	return regexp.Compile(expr)
}
