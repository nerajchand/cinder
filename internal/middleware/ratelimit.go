package middleware

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/cinder/cinder/internal/config"
)

const (
	// maxBuckets bounds limiter memory per route class.
	maxBuckets    = 100_000
	sweepInterval = time.Minute
)

type bucket struct {
	tokens float64
	last   time.Time
}

// limiter is a per-key token bucket with bounded memory. A bucket that has
// refilled to capacity is indistinguishable from a new one, so idle buckets
// are dropped by a periodic sweep.
type limiter struct {
	mu        sync.Mutex
	rate      float64 // tokens per second
	burst     float64
	buckets   map[netip.Addr]*bucket
	lastSweep time.Time
	now       func() time.Time
}

func newLimiter(perMin, burst int, now func() time.Time) *limiter {
	return &limiter{
		rate:    float64(perMin) / 60,
		burst:   float64(burst),
		buckets: make(map[netip.Addr]*bucket),
		now:     now,
	}
}

// allow consumes a token for key; if none is available it returns the wait
// until one is.
func (l *limiter) allow(key netip.Addr) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= sweepInterval {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxBuckets {
			l.evictOne()
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	} else if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens = math.Min(l.burst, b.tokens+el*l.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

func (l *limiter) sweep(now time.Time) {
	l.lastSweep = now
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// evictOne drops an arbitrary entry (map order is random) when the table is
// full of active buckets, trading one client's accounting for bounded memory.
func (l *limiter) evictOne() {
	for k := range l.buckets {
		delete(l.buckets, k)
		return
	}
}

func (l *limiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// limitKey normalises the client address: IPv6 clients share a bucket per /64.
func limitKey(a netip.Addr) netip.Addr {
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.Addr()
	}
	return a
}

// RateLimit applies per-IP token buckets, with separate (stricter) limits for
// the create, consume and status route classes taken from cfg. A per-minute
// value of 0 disables that class. Other routes are not limited. Client IP is
// the TCP peer unless the peer is in cfg.TrustedProxies, in which case
// X-Forwarded-For is used.
func RateLimit(cfg config.Config) Middleware {
	return rateLimit(cfg, time.Now)
}

func rateLimit(cfg config.Config, now func() time.Time) Middleware {
	limiters := map[routeClass]*limiter{}
	for class, perMin := range map[routeClass]int{
		classCreate:  cfg.RateLimitCreatePerMin,
		classConsume: cfg.RateLimitConsumePerMin,
		classStatus:  cfg.RateLimitStatusPerMin,
	} {
		if perMin > 0 {
			limiters[class] = newLimiter(perMin, cfg.RateLimitBurst, now)
		}
	}
	return func(next http.Handler) http.Handler {
		if len(limiters) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, class := classify(r)
			lim := limiters[class]
			if lim == nil {
				next.ServeHTTP(w, r)
				return
			}
			ip, ok := clientIP(r, cfg.TrustedProxies)
			if !ok {
				ip = netip.IPv6Unspecified() // unparseable peer: share one bucket
			}
			if allowed, wait := lim.allow(limitKey(ip)); !allowed {
				secs := int(math.Ceil(wait.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
