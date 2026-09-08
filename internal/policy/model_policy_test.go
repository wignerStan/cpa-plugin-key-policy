package policy

import (
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeModelPatterns(t *testing.T) {
	got, err := normalizeModelPatterns([]string{" gpt-5-* ", "Gpt-5-*", "", "claude-sonnet-4"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gpt-5-*", "claude-sonnet-4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeModelPatterns() = %v, want %v", got, want)
	}
	if _, err := normalizeModelPatterns([]string{"gpt-*mini"}); err == nil {
		t.Fatal("expected middle wildcard to be rejected")
	}
	if _, err := normalizeModelPatterns([]string{"gpt-**"}); err == nil {
		t.Fatal("expected multiple wildcards to be rejected")
	}
}

func TestModelPatternMatchesExactAndPrefix(t *testing.T) {
	tests := []struct {
		model   string
		pattern string
		want    bool
	}{
		{model: "gpt-5-mini", pattern: "gpt-5-*", want: true},
		{model: "GPT-5-MINI", pattern: "gpt-5-*", want: true},
		{model: "gpt-4o", pattern: "gpt-5-*", want: false},
		{model: "claude-sonnet-4", pattern: "claude-sonnet-4", want: true},
		{model: "claude-sonnet-4-5", pattern: "claude-sonnet-4", want: false},
		{model: "anything", pattern: "*", want: true},
	}
	for _, test := range tests {
		if got := modelPatternMatches(test.model, test.pattern); got != test.want {
			t.Errorf("modelPatternMatches(%q, %q) = %v, want %v", test.model, test.pattern, got, test.want)
		}
	}
}

func TestAuthenticateIncludeExcludeModelPolicy(t *testing.T) {
	plain := "cpa_model_policy"
	hash, err := HashKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.Configure(Config{
		Enabled:   true,
		StateFile: filepath.Join(t.TempDir(), "state.json"),
		Keys: []KeyConfig{{
			ID:            "models",
			Enabled:       true,
			KeyHash:       hash,
			IncludeModels: []string{"gpt-5-*", "claude-sonnet-4"},
			ExcludeModels: []string{"fast", "gpt-5-codex-*", "blocked-*"},
			Models: []ModelRule{
				{Alias: "fast", Provider: "codex", TargetModel: "gpt-5-mini"},
				{Alias: "mixed", Provider: "codex", TargetModel: "blocked-one"},
				{Alias: "mixed", Provider: "codex", TargetModel: "gpt-5-mini"},
				{Alias: "gpt-5-blocked-alias", Provider: "codex", TargetModel: "blocked-only"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"Authorization": {"Bearer " + plain}}

	excludedAlias := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"fast"}`))
	if excludedAlias.Allowed || excludedAlias.Reason != "model_excluded" {
		t.Fatalf("excluded explicit alias decision = %+v", excludedAlias)
	}

	nativePrefix := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"gpt-5-nano"}`))
	if !nativePrefix.Allowed || nativePrefix.Rule.Alias != "" {
		t.Fatalf("native prefix decision = %+v, want allowed pass-through", nativePrefix)
	}
	if _, _, routed := store.Route(headers, nil, "gpt-5-nano"); routed {
		t.Fatal("pattern-only native model must be left to CPA native routing")
	}

	nativeExact := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"claude-sonnet-4"}`))
	if !nativeExact.Allowed {
		t.Fatalf("exact include decision = %+v, want allowed", nativeExact)
	}

	excludedNative := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"gpt-5-codex-preview"}`))
	if excludedNative.Allowed || excludedNative.Reason != "model_excluded" {
		t.Fatalf("excluded native decision = %+v", excludedNative)
	}

	unmatched := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"gemini-2.5-pro"}`))
	if unmatched.Allowed || unmatched.Reason != "model_not_allowed" {
		t.Fatalf("unmatched decision = %+v", unmatched)
	}

	mixed := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"mixed"}`))
	if !mixed.Allowed || mixed.Rule.TargetModel != "gpt-5-mini" {
		t.Fatalf("mixed alias decision = %+v, want non-excluded target", mixed)
	}

	allTargetsExcluded := store.Authenticate("POST", "/v1/chat/completions", headers, nil, []byte(`{"model":"gpt-5-blocked-alias"}`))
	if allTargetsExcluded.Allowed || allTargetsExcluded.Reason != "model_excluded" {
		t.Fatalf("all-targets-excluded alias decision = %+v, want model_excluded without native fallback", allTargetsExcluded)
	}
}
