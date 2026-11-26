package nginx

import (
	"regexp"
	"strconv"
	"strings"
)

// Combined log format: ip - user [time] "METHOD path proto" status bytes
var logRegex = regexp.MustCompile(`^(\S+)\s+\S+\s+\S+\s+\[([^\]]+)\]\s+"([A-Z]+)\s+([^"\s]+)\s+[^"]*"\s+(\d{3})\s+(\d+)`)

// ParseDump parses `nginx -T` output. The dump is a concatenation of files,
// each introduced by "# configuration file <path>:". Every file is parsed with
// the same tokenizer as the linter, so nested and one-line blocks work, and
// each server/location/upstream keeps its file and line for click-to-source.
// Server and upstream blocks are collected wherever they appear (includes are
// printed as separate files by nginx -T).
func ParseDump(dump string) Analysis {
	a := Analysis{Servers: []ServerBlock{}, Upstreams: []UpstreamPool{}, Files: []string{}}
	for _, f := range splitDumpFiles(dump) {
		a.Files = append(a.Files, f.name)
		collectBlocks(ParseDirectives(f.body), f.name, &a)
	}
	a.ServerCount = len(a.Servers)
	a.UpstreamCount = len(a.Upstreams)
	return a
}

type dumpFile struct{ name, body string }

func splitDumpFiles(dump string) []dumpFile {
	const marker = "# configuration file "
	var files []dumpFile
	var cur *dumpFile
	var body []string
	flush := func() {
		if cur != nil {
			cur.body = strings.Join(body, "\n")
			files = append(files, *cur)
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		if strings.HasPrefix(line, marker) {
			flush()
			cur = &dumpFile{name: strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, marker)), ":")}
			body = nil
			continue
		}
		if cur == nil { // text before the first marker (e.g. "nginx: the configuration file ... syntax is ok")
			continue
		}
		body = append(body, line)
	}
	flush()
	if len(files) == 0 && strings.TrimSpace(dump) != "" { // plain config text, not a dump
		files = append(files, dumpFile{name: "", body: dump})
	}
	return files
}

func collectBlocks(nodes []*Directive, file string, a *Analysis) {
	for _, d := range nodes {
		switch {
		case d.Name == "server" && d.Block:
			a.Servers = append(a.Servers, buildServer(d, file))
		case d.Name == "upstream" && d.Block:
			up := UpstreamPool{Source: Source{file, d.Line}, Name: first(d.Args), Servers: []string{}}
			for _, c := range d.Children {
				if c.Name == "server" && !c.Block {
					up.Servers = append(up.Servers, strings.Join(c.Args, " "))
				}
			}
			a.Upstreams = append(a.Upstreams, up)
		case d.Block:
			collectBlocks(d.Children, file, a) // http {}, etc.
		}
	}
}

func buildServer(d *Directive, file string) ServerBlock {
	s := ServerBlock{Source: Source{file, d.Line}, Listen: []string{}, ServerName: []string{}, Locations: []LocationInfo{}, ProxyPasses: []string{}}
	for _, c := range d.Children {
		switch c.Name {
		case "listen":
			v := strings.Join(c.Args, " ")
			s.Listen = append(s.Listen, v)
			for _, arg := range c.Args {
				switch arg {
				case "ssl":
					s.SSL = true
				case "default_server", "default":
					s.DefaultServer = true
				}
			}
			if strings.HasSuffix(first(c.Args), "443") {
				s.SSL = true
			}
		case "server_name":
			s.ServerName = append(s.ServerName, c.Args...)
		case "root":
			s.Root = first(c.Args)
		case "index":
			s.Index = strings.Join(c.Args, " ")
		case "return":
			s.Return = strings.Join(c.Args, " ")
		case "location":
			collectLocations(c, file, &s)
		}
	}
	if len(s.Listen) == 0 {
		s.Listen = append(s.Listen, "80")
	}
	return s
}

// collectLocations flattens nested locations (nested ones are appended after their parent).
func collectLocations(d *Directive, file string, s *ServerBlock) {
	loc := LocationInfo{Source: Source{file, d.Line}, ProxyPass: []string{}, Path: strings.Join(d.Args, " ")}
	switch {
	case len(d.Args) >= 2:
		loc.Modifier, loc.Match = d.Args[0], d.Args[1]
	case len(d.Args) == 1 && strings.HasPrefix(d.Args[0], "@"):
		loc.Modifier, loc.Match = "@", d.Args[0][1:]
	case len(d.Args) == 1:
		loc.Match = d.Args[0]
	}
	var nested []*Directive
	for _, c := range d.Children {
		switch c.Name {
		case "proxy_pass":
			loc.ProxyPass = append(loc.ProxyPass, first(c.Args))
			s.ProxyPasses = append(s.ProxyPasses, first(c.Args))
		case "root":
			loc.Root = first(c.Args)
		case "alias":
			loc.Alias = first(c.Args)
		case "return":
			loc.Return = strings.Join(c.Args, " ")
		case "try_files":
			loc.TryFiles = strings.Join(c.Args, " ")
		case "location":
			nested = append(nested, c)
		}
	}
	s.Locations = append(s.Locations, loc)
	for _, n := range nested {
		collectLocations(n, file, s)
	}
}

func first(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

// ParseAccessLine extracts client IP, request path and status code from a
// combined-format access log line.
func ParseAccessLine(line string) (ip, path string, code int, ok bool) {
	m := logRegex.FindStringSubmatch(strings.TrimSpace(line))
	if len(m) < 6 {
		return "", "", 0, false
	}
	code, _ = strconv.Atoi(m[5])
	return m[1], m[4], code, true
}
