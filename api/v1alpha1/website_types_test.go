package v1alpha1

import (
	"testing"
)

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt32(i int32) *int32 {
	return &i
}

func TestInjectionSpecVersionPollingDefaults(t *testing.T) {
	// 1. Default (unspecified) -> Enabled, 30 seconds
	specDefault := InjectionSpec{}
	if !specDefault.IsVersionPollingEnabled() {
		t.Errorf("expected version polling to be enabled by default")
	}
	if specDefault.GetPollIntervalSeconds() != 30 {
		t.Errorf("expected default poll interval 30s, got %d", specDefault.GetPollIntervalSeconds())
	}

	// 2. Explicitly disabled via VersionPolling: false
	specDisabled := InjectionSpec{
		VersionPolling: ptrBool(false),
	}
	if specDisabled.IsVersionPollingEnabled() {
		t.Errorf("expected version polling to be disabled")
	}
	if specDisabled.GetPollIntervalSeconds() != 0 {
		t.Errorf("expected interval 0 when disabled, got %d", specDisabled.GetPollIntervalSeconds())
	}

	// 3. Explicitly disabled via PollIntervalSeconds: 0
	specZeroInterval := InjectionSpec{
		PollIntervalSeconds: ptrInt32(0),
	}
	if specZeroInterval.IsVersionPollingEnabled() {
		t.Errorf("expected version polling disabled when interval is 0")
	}
	if specZeroInterval.GetPollIntervalSeconds() != 0 {
		t.Errorf("expected interval 0, got %d", specZeroInterval.GetPollIntervalSeconds())
	}

	// 4. Explicitly enabled with custom interval
	specCustom := InjectionSpec{
		VersionPolling:      ptrBool(true),
		PollIntervalSeconds: ptrInt32(15),
	}
	if !specCustom.IsVersionPollingEnabled() {
		t.Errorf("expected version polling enabled")
	}
	if specCustom.GetPollIntervalSeconds() != 15 {
		t.Errorf("expected 15s interval, got %d", specCustom.GetPollIntervalSeconds())
	}
}

func TestInjectionSpecDeepCopy(t *testing.T) {
	spec := &InjectionSpec{
		Mode:                InjectionModeEndpoint,
		Path:                "/_config.js",
		VersionPolling:      ptrBool(false),
		PollIntervalSeconds: ptrInt32(45),
	}

	copied := spec.DeepCopy()
	if copied == nil {
		t.Fatalf("expected non-nil copy")
	}
	if *copied.VersionPolling != false {
		t.Errorf("expected copied VersionPolling to be false")
	}
	if *copied.PollIntervalSeconds != 45 {
		t.Errorf("expected copied PollIntervalSeconds to be 45")
	}

	// Mutate original and verify copied is unchanged
	*spec.VersionPolling = true
	*spec.PollIntervalSeconds = 60
	if *copied.VersionPolling != false {
		t.Errorf("mutation affected deepcopy VersionPolling")
	}
	if *copied.PollIntervalSeconds != 45 {
		t.Errorf("mutation affected deepcopy PollIntervalSeconds")
	}
}

func TestInjectionSpecVersionPath(t *testing.T) {
	// 1. Unset -> defaults to "/_version"
	sDefault := InjectionSpec{}
	if sDefault.GetVersionPath() != "/_version" {
		t.Errorf("expected default /_version, got: %s", sDefault.GetVersionPath())
	}

	// 2. Custom path
	sCustom := InjectionSpec{VersionPath: "/custom-version"}
	if sCustom.GetVersionPath() != "/custom-version" {
		t.Errorf("expected /custom-version, got: %s", sCustom.GetVersionPath())
	}
}

func TestSecuritySpecDeepCopy(t *testing.T) {
	orig := &SecuritySpec{
		Preset:             SecurityPresetStandard,
		ContentTypeOptions: ptrBool(true),
		HSTS: &HSTSSpec{
			Enabled: ptrBool(true),
			MaxAge:  ptrInt32(31536000),
		},
		PermissionsPolicy: &PermissionsPolicySpec{
			Camera: []string{"self"},
			Extra:  map[string][]string{"fullscreen": {"self"}},
		},
		CSP: &CSPSpec{
			Enabled: ptrBool(true),
			Directives: &CSPDirectives{
				ScriptSrc: []string{"'self'"},
			},
		},
		CustomHeaders: map[string]string{"X-Test": "1"},
	}

	copied := orig.DeepCopy()
	if copied == nil {
		t.Fatalf("expected non-nil copy")
	}

	*orig.ContentTypeOptions = false
	*orig.HSTS.MaxAge = 60
	orig.PermissionsPolicy.Camera[0] = "*"
	orig.PermissionsPolicy.Extra["fullscreen"][0] = "*"
	orig.CSP.Directives.ScriptSrc[0] = "'unsafe-inline'"
	orig.CustomHeaders["X-Test"] = "2"

	if !*copied.ContentTypeOptions {
		t.Errorf("mutation affected copied ContentTypeOptions")
	}
	if *copied.HSTS.MaxAge != 31536000 {
		t.Errorf("mutation affected copied HSTS.MaxAge")
	}
	if copied.PermissionsPolicy.Camera[0] != "self" {
		t.Errorf("mutation affected copied PermissionsPolicy.Camera")
	}
	if copied.PermissionsPolicy.Extra["fullscreen"][0] != "self" {
		t.Errorf("mutation affected copied PermissionsPolicy.Extra")
	}
	if copied.CSP.Directives.ScriptSrc[0] != "'self'" {
		t.Errorf("mutation affected copied CSP.Directives.ScriptSrc")
	}
	if copied.CustomHeaders["X-Test"] != "1" {
		t.Errorf("mutation affected copied CustomHeaders")
	}
}
