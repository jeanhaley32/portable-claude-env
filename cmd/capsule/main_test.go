package main

import "testing"

func TestResolveAPIKey(t *testing.T) {
	t.Run("flag value wins", func(t *testing.T) {
		t.Setenv("CAPSULE_API_KEY", "from-env")
		if got := resolveAPIKey("from-flag"); got != "from-flag" {
			t.Fatalf("resolveAPIKey(\"from-flag\") = %q, want \"from-flag\"", got)
		}
	})

	t.Run("falls back to env var when flag empty", func(t *testing.T) {
		t.Setenv("CAPSULE_API_KEY", "from-env")
		if got := resolveAPIKey(""); got != "from-env" {
			t.Fatalf("resolveAPIKey(\"\") = %q, want \"from-env\"", got)
		}
	})

	t.Run("empty when neither set", func(t *testing.T) {
		t.Setenv("CAPSULE_API_KEY", "")
		if got := resolveAPIKey(""); got != "" {
			t.Fatalf("resolveAPIKey(\"\") = %q, want \"\"", got)
		}
	})
}
