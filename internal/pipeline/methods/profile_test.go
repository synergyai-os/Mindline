package methods

import "testing"

func TestLoadBASBPARACODEProfile(t *testing.T) {
	profile, err := Load("basb-para-code")
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if profile.SchemaVersion != "method-profile/v0.1" {
		t.Fatalf("unexpected schema %q", profile.SchemaVersion)
	}
	if profile.MethodID != "basb-para-code" {
		t.Fatalf("unexpected method %q", profile.MethodID)
	}
	if profile.RunMode != "dry_run" {
		t.Fatalf("unexpected run mode %q", profile.RunMode)
	}
	if profile.Organize.DefaultModel != "PARA" {
		t.Fatalf("unexpected organize model %q", profile.Organize.DefaultModel)
	}
	if profile.ProcessorPolicy["youtube_url"].RequiredProcessor != "youtube_transcript" {
		t.Fatalf("unexpected YouTube processor")
	}
	if profile.ProcessorPolicy["youtube_url"].MissingArtifactReason != "missing_local_youtube_transcript" {
		t.Fatalf("unexpected YouTube missing-artifact reason")
	}
}

func TestLoadUnsupportedMethodFails(t *testing.T) {
	_, err := Load("zettelkasten")
	if err == nil {
		t.Fatalf("expected unsupported method error")
	}
	if err.Error() != "unsupported method: zettelkasten" {
		t.Fatalf("unexpected error: %v", err)
	}
}
