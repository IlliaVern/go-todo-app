package httpx

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// idleTTL is how long an unused client bucket is kept.
const idleTTL = 10 * time.Minute

// RateLimiter is an in-memory, per-client token bucket. Clients are keyed by
// the direct peer address (IPv6 by /64, since one host usually owns a whole
// prefix), so all requests arriving through one reverse proxy share a bucket.
// State is per process; run behind a shared limiter when scaling out.
type RateLimiter struct {
	limit rate.Limit
	burst int
	now   func() time.Time

	mu        sync.Mutex
	clients   map[netip.Addr]*client
	lastSweep time.Time
}

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter allows perMinute requests per client per minute, with bursts
// of up to perMinute.
func NewRateLimiter(perMinute int) *RateLimiter {
	perMinute = max(perMinute, 1)
	return &RateLimiter{
		limit:   rate.Limit(float64(perMinute) / 60),
		burst:   perMinute,
		now:     time.Now,
		clients: map[netip.Addr]*client{},
	}
}

// Middleware answers 429 with Retry-After once a client exhausts its bucket.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientKey(r)) {
			retry := int(math.Ceil(1 / float64(l.limit)))
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			Error(w, r, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(key netip.Addr) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) > idleTTL {
		for k, c := range l.clients {
			if now.Sub(c.lastSeen) > idleTTL {
				delete(l.clients, k)
			}
		}
		l.lastSweep = now
	}

	c, ok := l.clients[key]
	if !ok {
		c = &client{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.clients[key] = c
	}
	c.lastSeen = now
	return c.limiter.AllowN(now, 1)
}

// clientKey is the peer address, with IPv6 reduced to its /64 prefix.
func clientKey(r *http.Request) netip.Addr {
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	ip := addr.Addr().Unmap()
	if ip.Is6() {
		prefix, err := ip.Prefix(64)
		if err == nil {
			return prefix.Addr()
		}
	}
	return ip
}
