package main

import (
	"time"
)

const (
	primaryHardExcludeThreshold    = 0.95
	secondaryHardExcludeThreshold  = 0.99
)

func accountPrimaryUsageLocked(a *ProviderConnection) float64 {
	if a == nil {
		return 0
	}
	used := a.Usage.PrimaryUsedPercent
	if used == 0 {
		used = a.Usage.PrimaryUsed
	}
	return used
}

func accountSecondaryUsageLocked(a *ProviderConnection) float64 {
	if a == nil {
		return 0
	}
	used := a.Usage.SecondaryUsedPercent
	if used == 0 {
		used = a.Usage.SecondaryUsed
	}
	return used
}

func accountCoolingDownLocked(a *ProviderConnection, now time.Time) bool {
	if a == nil {
		return false
	}
	return !a.RateLimitUntil.IsZero() && a.RateLimitUntil.After(now)
}

func accountUsageExhaustedLocked(a *ProviderConnection) bool {
	if a == nil {
		return false
	}
	return accountPrimaryUsageLocked(a) >= primaryHardExcludeThreshold ||
		accountSecondaryUsageLocked(a) >= secondaryHardExcludeThreshold
}

func accountAvailableForRoutingLocked(a *ProviderConnection, now time.Time) bool {
	if a == nil {
		return false
	}
	if a.Dead || a.Disabled {
		return false
	}
	if accountCoolingDownLocked(a, now) {
		return false
	}
	if a.Type == AccountTypeBFL && a.Usage.HasCredits && a.Usage.CreditsBalance <= 0 {
		return false
	}
	return !accountUsageExhaustedLocked(a)
}
