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
	if providerConnections > attempts {
		attempts = providerConnections
	}
	if totalConnections > 0 && attempts > totalConnections {
		attempts = totalConnections
	}
	return attempts
}

func (policy RetryPolicy) CooldownWait(cooldown time.Duration) time.Duration {
	if cooldown <= 0 {
		return 0
	}
	if policy.MaxCooldownWait > 0 && cooldown > policy.MaxCooldownWait {
		return policy.MaxCooldownWait
	}
	return cooldown
}

func (policy RetryPolicy) ShouldRotate(class ErrorClass, ctx context.Context) bool {
	return ctx.Err() == nil && class.Retryable()
}

func (policy RetryPolicy) ShouldRetryBufferedCyberPolicy(pinned bool, attempt, attempts int, cyberAccess bool) bool {
	return pinned && !cyberAccess && attempt < attempts
}
