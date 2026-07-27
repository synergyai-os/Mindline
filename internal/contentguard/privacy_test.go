package contentguard

import "testing"

func TestContainsNonPersistableContent(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "credential", value: "password=synthetic-private-value"},
		{name: "signed URL", value: "read https://example.com/private?token=synthetic-private-value"},
		{name: "tracking URL mutation", value: "read https://example.com/public?utm_source=synthetic"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !ContainsNonPersistableContent(test.value) {
				t.Fatalf("unsafe content was accepted: %q", test.value)
			}
		})
	}
	if ContainsNonPersistableContent("safe title", "https://github.com/acme/tool") {
		t.Fatal("safe public content was rejected")
	}
}
