package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundJobsCancelAndJoin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := &backgroundJobs{}
	var exited atomic.Bool
	jobs.Go(ctx, func(ctx context.Context) { <-ctx.Done(); exited.Store(true) })
	cancel()
	done := make(chan struct{})
	go func() { jobs.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background jobs did not join after cancellation")
	}
	if !exited.Load() {
		t.Fatal("background job did not observe process cancellation")
	}
}
