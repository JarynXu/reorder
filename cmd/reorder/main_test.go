package main

import "testing"

func TestBuildVersionOverride(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })

	if got := buildVersion(); got != "v1.2.3" {
		t.Fatalf("buildVersion() = %q, want %q", got, "v1.2.3")
	}
}
