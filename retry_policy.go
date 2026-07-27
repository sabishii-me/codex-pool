package main

import (
	"context"
	"time"
)

// RetryPolicy owns request-level attempt and wait decisions. Account lifecycle
// side effects remain with the handler that has persistence and metrics access.
type RetryPolicy struct {
	ConfiguredAttempts int
	MaxCooldownWait    time.Duration
}

func (policy RetryPolicy) Attempts(providerConnections, totalConnections int) int {
	attempts := policy.ConfiguredAttempts
	if attempts <= 0 {
		attempts = 1
	}
	// ConfiguredAttempts is an operator safety cap, not a minimum. In
	// particular, PROXY_MAX_ATTEMPTS=1 must guarantee one upstream attempt.
	if providerConnections > 0 && attempts > providerConnections {
		attempts = providerConnections
	}
	if totalConnections > 0 && attempts > totalConnections {
		attempts = totalConnections
	}
	if attempts <= 0 {
		return 1
	}
	return attempts
}

func (policy RetryPolicy) CooldownWait(cooldown time.Duration) time.Duration {
	if cooldown <= 0 {
		return 0
	}
	if policy.MaxCooldownWait > 0 && cooldown > policy.MaxCooldownWait {
		return 0
	}
	return cooldown
}

func (policy RetryPolicy) ShouldRotate(class ErrorClass, ctx context.Context) bool {
	return ctx.Err() == nil && class.Retryable()
}

func (policy RetryPolicy) ShouldRetryBufferedCyberPolicy(pinned bool, attempt, attempts int, cyberAccess bool) bool {
	return pinned && !cyberAccess && attempt < attempts
}
