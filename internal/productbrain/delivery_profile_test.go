package productbrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDeliveryProfileRejectsUnknownAndSecretBearingFields(t *testing.T) {
	profile := testDeliveryProfile()
	base, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var baseObject map[string]any
	if err := json.Unmarshal(base, &baseObject); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"credential value", func(value map[string]any) { value["credential"].(map[string]any)["value"] = "sentinel-secret" }},
		{"top-level api key", func(value map[string]any) { value["api_key"] = "sentinel-secret" }},
		{"unknown innocuous field", func(value map[string]any) { value["future_field"] = true }},
		{"secret marker in allowed field", func(value map[string]any) { value["profile_id"] = "pb_sk_SENTINEL_ONLY" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value map[string]any
			encoded, _ := json.Marshal(baseObject)
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			test.mutate(value)
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "profile.json")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDeliveryProfile(path); err == nil {
				t.Fatal("unsafe delivery profile was accepted")
			}
		})
	}
}

func TestLoadDeliveryProfileRejectsTrailingJSONAndAcceptsStrictProfile(t *testing.T) {
	profile := testDeliveryProfile()
	body, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(t.TempDir(), "valid.json")
	if err := os.WriteFile(validPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeliveryProfile(validPath); err != nil {
		t.Fatalf("strict profile was rejected: %v", err)
	}
	trailingPath := filepath.Join(t.TempDir(), "trailing.json")
	if err := os.WriteFile(trailingPath, append(body, []byte("\n{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeliveryProfile(trailingPath); err == nil {
		t.Fatal("trailing delivery profile JSON was accepted")
	}
}
