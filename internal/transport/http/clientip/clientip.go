// Package clientip resolves the originating client IP for an inbound HTTP
// request. It replaces gin.Context.ClientIP() in the chi-based handler
// layer.
//
// Resolution order honours the trusted-proxy boundary: header values are
// trusted only when the immediate transport peer (r.RemoteAddr) sits in
// one of the configured trusted CIDRs. This blocks header spoofing from
// untrusted clients while preserving the real client IP when a known
// reverse proxy or CDN is fronting the service.
//
// Order of evaluation when the peer is trusted:
//  1. The left-most public IP in X-Forwarded-For. Private/loopback hops
//     between the edge and the origin are skipped over (matches the
//     net/http stdlib trust model and Cloudflare's documented behaviour).
//  2. X-Real-IP — single-value fallback used by some proxies.
//  3. The host portion of r.RemoteAddr.
//
// When the peer is not trusted (or no trusted CIDRs are configured), the
// headers are ignored and only r.RemoteAddr is consulted.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// Resolver carries the trusted-proxy CIDR set used to validate forwarded
// headers. Construct one at process start via New and reuse it across
// requests; the zero value is also usable but treats every peer as
// untrusted (headers ignored, RemoteAddr always wins).
type Resolver struct {
	trusted []*net.IPNet
}

// New returns a Resolver that trusts header values from peers inside any
// of the supplied CIDRs. CIDRs that fail to parse are silently dropped —
// callers that want strictness should pre-validate the input.
func New(trustedCIDRs []string) *Resolver {
	r := &Resolver{trusted: make([]*net.IPNet, 0, len(trustedCIDRs))}
	for _, c := range trustedCIDRs {
		if _, ipnet, err := net.ParseCIDR(c); err == nil {
			r.trusted = append(r.trusted, ipnet)
		}
	}
	return r
}

// From resolves the client IP for r. Always returns a non-empty string in
// well-formed requests; falls back to r.RemoteAddr verbatim when no useful
// address can be extracted.
func (rv *Resolver) From(r *http.Request) string {
	peer := remoteHost(r.RemoteAddr)

	if rv.trusts(peer) {
		if ip := firstPublicXFF(r.Header.Get("X-Forwarded-For")); ip != "" {
			return ip
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}

	return peer
}

// trusts reports whether the supplied IP literal is inside one of the
// resolver's configured trusted networks.
func (rv *Resolver) trusts(ip string) bool {
	if len(rv.trusted) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range rv.trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// remoteHost strips the :port suffix from a `host:port` literal. Returns
// the input unchanged on malformed values so the resolver still produces
// a deterministic string for logging.
func remoteHost(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// firstPublicXFF returns the first non-private IP in a comma-separated
// X-Forwarded-For header, working left-to-right. Private/loopback/link-
// local hops between the edge and the origin are skipped so the returned
// value reflects the real client. Returns "" when every entry is private
// or the header is empty.
func firstPublicXFF(header string) string {
	for raw := range strings.SplitSeq(header, ",") {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		if isPrivate(parsed) {
			continue
		}
		return ip
	}
	return ""
}

// isPrivate reports whether an IP is in a non-routable range (RFC 1918,
// loopback, link-local, ULA, or unspecified). Used to skip over
// internal-network hops in X-Forwarded-For chains.
func isPrivate(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	return false
}
