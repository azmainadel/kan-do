package web

import (
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// csp allows scripts only from this server, so even a bug in the Markdown renderer
// cannot run injected script, and requests only back to this server.
const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// guard decides which Host headers the board answers to.
//
// DNS rebinding needs a domain name the attacker controls, so the board only accepts
// localhost, IP addresses and, when it listens on the network, this machine's own
// names. It never accepts other domain names.
type guard struct {
	port uint16
	lan  bool
	// names are this machine's names, like pi and pi.local; filled only when lan.
	names []string
}

func newGuard(port uint16, lan bool) *guard {
	g := &guard{port: port, lan: lan}
	if !lan {
		return g
	}
	run := func(name string, args ...string) string {
		out, err := exec.Command(name, args...).Output()
		if err != nil {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(string(out)))
	}
	// macOS keeps its Bonjour name apart; hostname there can be a DHCP address.
	for _, full := range []string{run("scutil", "--get", "LocalHostName"), run("hostname")} {
		if full == "" || net.ParseIP(full) != nil {
			continue
		}
		short, _, _ := strings.Cut(full, ".")
		for _, name := range []string{short + ".local", full, short} {
			if !slices.Contains(g.names, name) {
				g.names = append(g.names, name)
			}
		}
	}
	return g
}

func (g *guard) allows(host string) bool {
	name, port := host, ""
	if i := strings.LastIndexByte(host, ':'); i >= 0 && !strings.Contains(host[i:], "]") {
		name, port = host[:i], host[i+1:]
	}
	if port != "" && port != strconv.Itoa(int(g.port)) {
		return false
	}
	name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(name, "["), "]"), ".")
	ip, err := netip.ParseAddr(name)
	isIP := err == nil
	if strings.EqualFold(name, "localhost") || isIP && ip.IsLoopback() {
		return true
	}
	return g.lan && (isIP || slices.ContainsFunc(g.names, func(n string) bool { return strings.EqualFold(n, name) }))
}

// protect refuses requests for a Host this board does not answer to, and browser
// requests sent from any page other than the board itself. It adds security headers
// to every reply.
func (g *guard) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-cache")
		origin := r.Header.Get("Origin")
		if !g.allows(r.Host) || origin != "" && !strings.EqualFold(origin, "http://"+r.Host) {
			http.Error(w, "kan-do does not answer requests from other sites", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
