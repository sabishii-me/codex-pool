package main

// Compile-time capability checks keep the compatibility Provider composition
// honest while focused consumers migrate to narrower interfaces.
var (
	_ ProviderCredentialLoader  = (*CodexProvider)(nil)
	_ ProviderAuthenticator     = (*CodexProvider)(nil)
	_ ProviderRefresher         = (*CodexProvider)(nil)
	_ ProviderUsageParser       = (*CodexProvider)(nil)
	_ ProviderUsageHeaderParser = (*CodexProvider)(nil)
	_ ProviderRouteTarget       = (*CodexProvider)(nil)
	_ ProviderStreamDetector    = (*CodexProvider)(nil)

	_ Provider = (*ClaudeProvider)(nil)
	_ Provider = (*GeminiProvider)(nil)
	_ Provider = (*AntigravityProvider)(nil)
	_ Provider = (*GrokProvider)(nil)
	_ Provider = (*DeclarativeProvider)(nil)
)
