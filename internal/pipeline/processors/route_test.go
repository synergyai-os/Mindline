package processors

import (
	"testing"

	"github.com/synergyai-os/Mindline/internal/pipeline/methods"
)

func TestPlanProcessorsForFixtureCandidates(t *testing.T) {
	profile, err := methods.Load("basb-para-code")
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	cases := []struct {
		name     string
		text     string
		urls     []URLUnit
		private  bool
		secret   bool
		steps    []string
		blockers []string
	}{
		{"text", "Mindline should keep raw capture, method policy, and destination preview separate.", nil, false, false, []string{"text_capture_review:required:planned"}, nil},
		{"youtube", "Watch https://www.youtube.com/watch?v=wp6example", []URLUnit{{URL: "https://www.youtube.com/watch?v=wp6example", Kind: "youtube_url"}}, false, false, []string{"youtube_transcript:required:planned", "manual_processing_required:blocked:blocked"}, []string{"missing_local_youtube_transcript"}},
		{"linkedinWeb", "https://www.linkedin.com/posts/example-mindline https://example.com/mindline-routing", []URLUnit{{URL: "https://www.linkedin.com/posts/example-mindline", Kind: "linkedin_url"}, {URL: "https://example.com/mindline-routing", Kind: "web_url"}}, false, false, []string{"linkedin_post_context:required:planned", "web_page_metadata:required:planned", "manual_processing_required:blocked:blocked"}, []string{"missing_local_linkedin_context", "missing_local_web_metadata"}},
		{"unknown", "unclassified://mindline/local-capture", []URLUnit{{URL: "unclassified://mindline/local-capture", Kind: "unknown"}}, false, false, []string{"manual_processing_required:blocked:blocked"}, []string{"unknown_content_type"}},
		{"private", "PRIVATE_DM_SENTINEL_DO_NOT_WRITE", nil, true, false, []string{"private_provenance_block:blocked:blocked"}, []string{"private_provenance_requires_review"}},
		{"secret", "sk-test-secret-do-not-leak", nil, false, true, []string{"secret_skip:blocked:blocked"}, []string{"secret_like_content_detected"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := Plan(Input{Text: tc.text, URLs: tc.urls, PrivateProvenance: tc.private, SecretLike: tc.secret}, profile, []string{"DEC-15", "DEC-6", "DEC-12", "DEC-13"})
			if got := compactSteps(plan.Steps); !equal(got, tc.steps) {
				t.Fatalf("steps got %v want %v", got, tc.steps)
			}
			if !equal(plan.Blockers, tc.blockers) {
				t.Fatalf("blockers got %v want %v", plan.Blockers, tc.blockers)
			}
		})
	}
}

func compactSteps(steps []Step) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.ProcessorID+":"+step.Requirement+":"+step.Status)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
