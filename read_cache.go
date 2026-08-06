package main

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ttlSingleFlight deduplicates concurrent identical expensive read-only
// computations and serves them for a short TTL. The first request computes;
// concurrent callers wait for and reuse that result. This stops N users opening
// Usage from issuing N identical heavy aggregations against the analytics store.
type ttlSingleFlight struct {
	mu      sync.Mutex
	entries map[string]*ttlSFEntry
}

type ttlSFEntry struct {
	status   int
	header   http.Header
	body     []byte
	expires  time.Time
	inflight bool
	waiters  []chan struct{}
}

func newTTLSingleFlight() *ttlSingleFlight {
	return &ttlSingleFlight{entries: make(map[string]*ttlSFEntry)}
}

func (c *ttlSingleFlight) Get(key string, ttl time.Duration, compute func() (int, http.Header, []byte)) (int, http.Header, []byte) {
	c.mu.Lock()
	now := time.Now()
	if e, ok := c.entries[key]; ok {
		if !e.inflight && now.Before(e.expires) && e.body != nil {
			c.mu.Unlock()
			return e.status, e.header, e.body
		}
		if e.inflight {
			ch := make(chan struct{})
			e.waiters = append(e.waiters, ch)
			c.mu.Unlock()
			<-ch
			c.mu.Lock()
			e2, ok := c.entries[key]
			c.mu.Unlock()
			if ok && !e2.inflight && e2.body != nil {
				return e2.status, e2.header, e2.body
			}
			// Entry vanished/expired while waiting; recompute.
			return c.Get(key, ttl, compute)
		}
	}
	// Become the in-flight leader (also overwrites a stale expired entry).
	entry := &ttlSFEntry{inflight: true}
	c.entries[key] = entry
	c.mu.Unlock()

	status, header, body := compute()

	c.mu.Lock()
	entry.status, entry.header, entry.body = status, header, body
	entry.expires = now.Add(ttl)
	entry.inflight = false
	waiters := entry.waiters
	entry.waiters = nil
	c.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
	return status, header, body
}

// cachedRecorder captures an HTTP response so expensive handlers can be
// computed once and replayed to concurrent readers.
type cachedRecorder struct {
	status int
	header http.Header
	body   bytes.Buffer
}

func (r *cachedRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *cachedRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *cachedRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

// cachedHandler wraps an expensive read-only handler with a TTL single-flight
// cache. cacheable decides whether this particular request may be served from
// the shared cache (used to keep per-user scopes out of the cache). The cache
// runs after authorization, so cache entries are only created for signed-in
// requests and never leak between unauthenticated callers.
func (api *DataAPI) cachedHandler(ttl time.Duration, cacheable func(*http.Request) bool, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if api.readCache == nil || !cacheable(r) {
			handler(w, r)
			return
		}
		key := r.URL.Path + "|" + r.URL.RawQuery
		status, header, body := api.readCache.Get(key, ttl, func() (int, http.Header, []byte) {
			rec := &cachedRecorder{header: make(http.Header)}
			handler(rec, r)
			return rec.status, rec.header, rec.body.Bytes()
		})
		for name, values := range header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

func cacheAllGET(*http.Request) bool { return true }

func cachePoolUsageOnly(r *http.Request) bool {
	return r != nil && strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("scope")), "pool")
}
