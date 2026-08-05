package main

import "strings"

// WorkloadKind is an operation class, not an LLM capability. In particular,
// accepting image input never makes a text model eligible for image generation.
type WorkloadKind string

const (
	WorkloadTextGeneration  WorkloadKind = "text_generation"
	WorkloadImageGeneration WorkloadKind = "image_generation"
	WorkloadEmbedding       WorkloadKind = "embedding"
	WorkloadAudio           WorkloadKind = "audio"
	WorkloadVideo           WorkloadKind = "video"
)

type NativeModelDescriptor struct {
	ID               string       `json:"id"`
	ProviderID       ProviderID   `json:"provider"`
	UpstreamID       string       `json:"upstream_id"`
	Kind             WorkloadKind `json:"model_kind"`
	InputModalities  []string     `json:"input_modalities"`
	OutputModalities []string     `json:"output_modalities"`
	OutputMIMETypes  []string     `json:"output_mime_types,omitempty"`
	Provenance       string       `json:"capability_provenance"`
	ProvenanceURL    string       `json:"capability_provenance_url"`
	VerifiedAt       string       `json:"capability_verified_at"`
}

var nativeImageModels = []NativeModelDescriptor{
	{ID: "bfl/flux-2-pro", ProviderID: AccountTypeBFL, UpstreamID: "flux-2-pro", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/jpeg", "image/png"}, Provenance: "official_provider_openapi", ProvenanceURL: "https://docs.bfl.ml/api-reference/models/generate-or-edit-an-image-with-flux2-[pro].md", VerifiedAt: "2026-08-02"},
	{ID: "bfl/flux-2-flex", ProviderID: AccountTypeBFL, UpstreamID: "flux-2-flex", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/jpeg", "image/png"}, Provenance: "official_provider_openapi", ProvenanceURL: "https://api.bfl.ai/openapi.json", VerifiedAt: "2026-08-02"},
	{ID: "bfl/flux-2-max", ProviderID: AccountTypeBFL, UpstreamID: "flux-2-max", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/jpeg", "image/png"}, Provenance: "official_provider_openapi", ProvenanceURL: "https://api.bfl.ai/openapi.json", VerifiedAt: "2026-08-02"},
	{ID: "bfl/flux-2-klein-4b", ProviderID: AccountTypeBFL, UpstreamID: "flux-2-klein-4b", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/jpeg", "image/png"}, Provenance: "official_provider_openapi", ProvenanceURL: "https://api.bfl.ai/openapi.json", VerifiedAt: "2026-08-02"},
	{ID: "google-ai-image/gemini-2.5-flash-image", ProviderID: AccountTypeGoogleAIImage, UpstreamID: "gemini-2.5-flash-image", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/png", "image/jpeg"}, Provenance: "official_provider_documentation", ProvenanceURL: "https://ai.google.dev/gemini-api/docs/image-generation", VerifiedAt: "2026-08-02"},
	{ID: "google-ai-image/gemini-3-pro-image-preview", ProviderID: AccountTypeGoogleAIImage, UpstreamID: "gemini-3-pro-image-preview", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/png", "image/jpeg"}, Provenance: "official_provider_documentation", ProvenanceURL: "https://ai.google.dev/gemini-api/docs/image-generation", VerifiedAt: "2026-08-02"},
	{ID: "google-ai-image/gemini-3.1-flash-image-preview", ProviderID: AccountTypeGoogleAIImage, UpstreamID: "gemini-3.1-flash-image-preview", Kind: WorkloadImageGeneration, InputModalities: []string{"text"}, OutputModalities: []string{"image"}, OutputMIMETypes: []string{"image/png", "image/jpeg"}, Provenance: "official_provider_documentation", ProvenanceURL: "https://ai.google.dev/gemini-api/docs/image-generation", VerifiedAt: "2026-08-02"},
}

func resolveNativeModel(id string, kind WorkloadKind) (NativeModelDescriptor, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, model := range nativeImageModels {
		if strings.ToLower(model.ID) == id && model.Kind == kind {
			return model, true
		}
	}
	return NativeModelDescriptor{}, false
}
