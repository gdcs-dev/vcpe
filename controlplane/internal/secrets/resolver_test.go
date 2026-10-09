package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func TestResolveAndValidateWirelessPassphraseProviders(t *testing.T) {
	t.Setenv("VCPE_TEST_WIFI_PASSPHRASE", "valid env passphrase")
	secretFile := filepath.Join(t.TempDir(), "secrets.env")
	if err := os.WriteFile(secretFile, []byte("WIFI=valid file passphrase\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	refs := []manifest.SecretRef{
		{Name: "env-wifi", Provider: "env", Key: "VCPE_TEST_WIFI_PASSPHRASE"},
		{Name: "file-wifi", Provider: "file", Key: secretFile + ":WIFI"},
		{Name: "literal-wifi", Provider: "literal", Value: "valid literal passphrase"},
	}
	resolved, err := Resolve(refs)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	networks := []manifest.WirelessNetwork{
		{Security: manifest.WirelessWPA2Personal, PassphraseSecretRef: "env-wifi"},
		{Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "file-wifi"},
		{Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "literal-wifi"},
	}
	if err := ValidateWirelessPassphrases(networks, resolved); err != nil {
		t.Fatalf("ValidateWirelessPassphrases() error = %v", err)
	}
}

func TestValidateWirelessPassphrasesRejectsInvalidValuesWithoutDisclosure(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "short", value: "short", want: "invalid length"},
		{name: "long", value: strings.Repeat("x", 64), want: "invalid length"},
		{name: "newline", value: "password\n", want: "non-printable ASCII"},
		{name: "non ASCII", value: "password-\u00e9", want: "non-printable ASCII"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			networks := []manifest.WirelessNetwork{{
				Security:            manifest.WirelessWPA3Personal,
				PassphraseSecretRef: "home-wifi",
			}}
			err := ValidateWirelessPassphrases(networks, map[string]string{"home-wifi": test.value})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateWirelessPassphrases() error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("error disclosed credential value: %q", err)
			}
		})
	}
}

func TestValidateWirelessPassphrasesIgnoresOpenNetworks(t *testing.T) {
	networks := []manifest.WirelessNetwork{{Security: manifest.WirelessOpen}}
	if err := ValidateWirelessPassphrases(networks, nil); err != nil {
		t.Fatalf("ValidateWirelessPassphrases() error = %v", err)
	}
}

func TestValidateMeshPassphrasesRejectsMissingOrUnsafeValues(t *testing.T) {
	services := []manifest.Service{{Name: "root", Radios: []manifest.Radio{{Mode: manifest.RadioModeMesh, Mesh: &manifest.Mesh{ID: "mesh-home", SAESecretRef: "mesh-key"}}}}}
	for _, test := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"missing", nil, "mesh-key"},
		{"short", map[string]string{"mesh-key": "short"}, "invalid length"},
		{"newline", map[string]string{"mesh-key": "long-password\nunsafe"}, "non-printable"},
		{"valid", map[string]string{"mesh-key": "long-password"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateMeshPassphrases(services, test.values)
			if test.want == "" && err != nil || test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("mesh validation error = %v, want %q", err, test.want)
			}
			if err != nil && strings.Contains(err.Error(), "long-password") {
				t.Fatalf("mesh validation leaked secret value: %v", err)
			}
		})
	}
}
