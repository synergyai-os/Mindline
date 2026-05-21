package methods

import "fmt"

type Profile struct {
	SchemaVersion   string                     `json:"schema_version"`
	MethodID        string                     `json:"method_id"`
	RunMode         string                     `json:"run_mode"`
	Collect         CollectPolicy              `json:"collect"`
	Organize        OrganizePolicy             `json:"organize"`
	Distill         DistillPolicy              `json:"distill"`
	Express         ExpressPolicy              `json:"express"`
	ProcessorPolicy map[string]ProcessorPolicy `json:"processor_policy"`
}

type CollectPolicy struct {
	KeepRawCapture bool `json:"keep_raw_capture"`
}

type OrganizePolicy struct {
	DefaultModel string `json:"default_model"`
}

type DistillPolicy struct {
	Sections []string `json:"sections"`
}

type ExpressPolicy struct {
	DefaultDestination string `json:"default_destination"`
}

type ProcessorPolicy struct {
	RequiredProcessor     string `json:"required_processor"`
	MissingArtifactReason string `json:"missing_artifact_reason"`
}

func Load(methodID string) (Profile, error) {
	if methodID != "basb-para-code" {
		return Profile{}, fmt.Errorf("unsupported method: %s", methodID)
	}
	return Profile{
		SchemaVersion: "method-profile/v0.1",
		MethodID:      "basb-para-code",
		RunMode:       "dry_run",
		Collect:       CollectPolicy{KeepRawCapture: true},
		Organize:      OrganizePolicy{DefaultModel: "PARA"},
		Distill: DistillPolicy{Sections: []string{
			"Snapshot",
			"Source Content",
			"Key Details",
			"Relevance",
			"Signals",
			"Related Sources",
			"Next Action",
		}},
		Express: ExpressPolicy{DefaultDestination: "tolaria"},
		ProcessorPolicy: map[string]ProcessorPolicy{
			"text":         {RequiredProcessor: "text_capture_review"},
			"youtube_url":  {RequiredProcessor: "youtube_transcript", MissingArtifactReason: "missing_local_youtube_transcript"},
			"linkedin_url": {RequiredProcessor: "linkedin_post_context", MissingArtifactReason: "missing_local_linkedin_context"},
			"web_url":      {RequiredProcessor: "web_page_metadata", MissingArtifactReason: "missing_local_web_metadata"},
			"pdf_url":      {RequiredProcessor: "pdf_text_extract", MissingArtifactReason: "missing_local_pdf_text"},
			"unknown":      {RequiredProcessor: "manual_processing_required", MissingArtifactReason: "unknown_content_type"},
		},
	}, nil
}
