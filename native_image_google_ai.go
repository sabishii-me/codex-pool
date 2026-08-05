package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/url"
	"strings"
)

func executeGoogleAIImage(ctx context.Context, transport http.RoundTripper, provider *GoogleAIImageProvider, connection *ProviderConnection, model NativeModelDescriptor, input nativeImageRequest) (nativeImageResult, error) {
	payload := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": input.Prompt}}}},
		"generationConfig": map[string]any{"responseModalities": []string{"IMAGE"}},
	}
	raw, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(provider.base.String(), "/") + "/v1beta/models/" + url.PathEscape(model.UpstreamID) + ":generateContent"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	provider.SetAuthHeaders(req, connection)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nativeImageResult{}, err
	}
	defer resp.Body.Close()
	body, err := readBoundedBody(resp.Body, 44<<20, "Google AI image response")
	if err != nil {
		return nativeImageResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nativeImageResult{}, &nativeImageUpstreamError{Provider: "Google AI", Operation: "generation", StatusCode: resp.StatusCode, Headers: resp.Header.Clone()}
	}
	var result struct {
		ResponseID string `json:"responseId"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MIMEType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nativeImageResult{}, errors.New("invalid Google AI image response")
	}
	for _, candidate := range result.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil {
				continue
			}
			mime := strings.ToLower(strings.TrimSpace(part.InlineData.MIMEType))
			if mime != "image/png" && mime != "image/jpeg" {
				return nativeImageResult{}, fmt.Errorf("unsupported Google AI image MIME type %q", mime)
			}
			data, decodeErr := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if decodeErr != nil || len(data) == 0 {
				return nativeImageResult{}, errors.New("Google AI returned invalid image data")
			}
			config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
			if decodeErr != nil {
				return nativeImageResult{}, errors.New("Google AI returned invalid image bytes")
			}
			return nativeImageResult{Bytes: data, MIME: mime, Width: config.Width, Height: config.Height, OperationID: result.ResponseID}, nil
		}
	}
	return nativeImageResult{}, errors.New("Google AI response omitted native image output")
}
