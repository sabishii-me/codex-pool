package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTTLSingleFlightComputesOnceForConcurrentReaders(t *testing.T) {
	cache := newTTLSingleFlight()
	var computes atomic.Int32
	compute := func() (int, http.Header, []byte) {
		computes.Add(1)
		time.Sleep(20 * time.Millisecond) // force overlap
		h := make(http.Header)
		h.Set("Content-Type", "application/json")
		return 200, h, []byte(`{"ok":true}`)
	}
	const workers = 32
	var wg sync.WaitGroup
	statuses := make([]int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, _, _ := cache.Get("usage|scope=pool", 5*time.Second, compute)
			statuses[i] = status
		}(i)
	}
	wg.Wait()
	if got := computes.Load(); got != 1 {
		t.Fatalf("compute ran %d times, want 1 (single flight)", got)
	}
	for i, status := range statuses {
		if status != 200 {
			t.Fatalf("worker %d status = %d, want 200", i, status)
		}
	}
}

func TestTTLSingleFlightServesCachedWithinTTLAndRefreshesAfter(t *testing.T) {
	cache := newTTLSingleFlight()
	var computes atomic.Int32
	compute := func() (int, http.Header, []byte) {
		n := computes.Add(1)
		return 200, nil, []byte{byte('a' + n - 1)}
	}
	_, _, body := cache.Get("k", 50*time.Millisecond, compute)
	if string(body) != "a" {
		t.Fatalf("first body = %q, want a", body)
	}
	_, _, body = cache.Get("k", 50*time.Millisecond, compute)
	if string(body) != "a" {
		t.Fatalf("cached body = %q, want a (cached)", body)
	}
	time.Sleep(60 * time.Millisecond)
	_, _, body = cache.Get("k", 50*time.Millisecond, compute)
	if string(body) != "b" {
		t.Fatalf("refreshed body = %q, want b", body)
	}
	if got := computes.Load(); got != 2 {
		t.Fatalf("compute ran %d times, want 2", got)
	}
}

func TestCachedHandlerSkipsCacheWhenNotCacheable(t *testing.T) {
	api := &DataAPI{readCache: newTTLSingleFlight()}
	var calls atomic.Int32
	inner := func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}
	handler := api.cachedHandler(time.Second, cachePoolUsageOnly, inner)
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		handler(rr, httptest.NewRequest(http.MethodGet, "/api/v2/usage?scope=me", nil))
		if rr.Code != 200 {
			t.Fatalf("status = %d", rr.Code)
		}
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("non-cacheable scope ran %d times, want 3 (never cached)", got)
	}
}

func TestCachedHandlerCachesPoolScopeAcrossReaders(t *testing.T) {
	api := &DataAPI{readCache: newTTLSingleFlight()}
	var calls atomic.Int32
	inner := func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"scope":"pool"}`))
	}
	handler := api.cachedHandler(time.Second, cachePoolUsageOnly, inner)
	var wg sync.WaitGroup
	statuses := make([]int, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			handler(rr, httptest.NewRequest(http.MethodGet, "/api/v2/usage?scope=pool&hours=24", nil))
			statuses[i] = rr.Code
		}(i)
	}
	wg.Wait()
	for i, status := range statuses {
		if status != 200 {
			t.Fatalf("reader %d status = %d, want 200", i, status)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("pool scope computed %d times, want 1 (cached)", got)
	}
}
