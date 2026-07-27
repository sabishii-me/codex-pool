package main

import "log"

// writeImageGenerationTrace records failure metadata only. Raw provider streams,
// assembled outputs, prompts, and artifacts are never persisted.
func (h *proxyHandler) writeImageGenerationTrace(reqID string, account *ProviderConnection, rawStream, assembled []byte, cause error) {
	accountID := ""
	if account != nil {
		accountID = hashAccountID(account.ID)
	}
	log.Printf("[%s] image generation failed account=%s raw_bytes=%d assembled_bytes=%d error=%v", reqID, accountID, len(rawStream), len(assembled), cause)
}
