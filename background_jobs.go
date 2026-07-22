package main

import (
	"context"
	"sync"
)

// backgroundJobs owns process-lifetime goroutines so shutdown can cancel and
// join them before persistence stores are closed.
type backgroundJobs struct{ waitGroup sync.WaitGroup }

func (jobs *backgroundJobs) Go(ctx context.Context, run func(context.Context)) {
	if jobs == nil || run == nil {
		return
	}
	jobs.waitGroup.Add(1)
	go func() { defer jobs.waitGroup.Done(); run(ctx) }()
}

func (jobs *backgroundJobs) Wait() {
	if jobs != nil {
		jobs.waitGroup.Wait()
	}
}
