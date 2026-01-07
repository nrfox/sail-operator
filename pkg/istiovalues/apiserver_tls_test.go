// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package istiovalues

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	configv1 "github.com/openshift/api/config/v1"

	"github.com/istio-ecosystem/sail-operator/pkg/helm"
)

func TestGetTLSSettingsFromAPIServer(t *testing.T) {
	tests := []struct {
		name      string
		apiServer *configv1.APIServer
		expected  TLSSettings
	}{
		{
			name: "custom TLS profile with ciphers",
			apiServer: &configv1.APIServer{
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
							},
						},
					},
				},
			},
			expected: TLSSettings{
				CipherSuites: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTLSSettingsFromAPIServer(tt.apiServer)
			if diff := cmp.Diff(result.CipherSuites, tt.expected.CipherSuites); diff != "" {
				t.Errorf("unexpected cipher suites; diff (-expected, +actual):\n%v", diff)
			}
		})
	}
}

func TestApplyAPIServerTLSSettings(t *testing.T) {
	tests := []struct {
		name        string
		tlsSettings *TLSSettings
		inputValues helm.Values
		checkKey    string
		checkValue  any
		shouldError bool
	}{
		{
			name:        "nil TLS settings",
			tlsSettings: nil,
			inputValues: helm.Values{},
			checkKey:    "",
			checkValue:  nil,
			shouldError: false,
		},
		{
			name: "applies cipher suites to tlsDefaults",
			tlsSettings: &TLSSettings{
				CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			},
			inputValues: helm.Values{},
			checkKey:    "meshConfig.tlsDefaults.cipherSuites",
			checkValue:  []any{"ECDHE-RSA-AES128-GCM-SHA256"},
			shouldError: false,
		},
		{
			name: "does not override existing cipherSuites",
			tlsSettings: &TLSSettings{
				CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			},
			inputValues: helm.Values{
				"meshConfig": map[string]any{
					"tlsDefaults": map[string]any{
						"cipherSuites": []any{"TLS_AES_128_GCM_SHA256"},
					},
				},
			},
			checkKey:    "meshConfig.tlsDefaults.cipherSuites",
			checkValue:  []any{"TLS_AES_128_GCM_SHA256"}, // Should keep original value
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ApplyAPIServerTLSSettings(tt.tlsSettings, tt.inputValues)
			if (err != nil) != tt.shouldError {
				t.Errorf("ApplyAPIServerTLSSettings() error = %v, shouldError = %v", err, tt.shouldError)
				return
			}

			if tt.checkKey != "" {
				val, found, err := result.GetString(tt.checkKey)
				if err == nil && found {
					if strVal, ok := tt.checkValue.(string); ok && val != strVal {
						t.Errorf("Value at %q = %q, want %q", tt.checkKey, val, strVal)
					}
				}
			}
		})
	}
}

func TestApplyAPIServerTLSSettings_ExtraContainerArgs(t *testing.T) {
	tests := []struct {
		name                 string
		tlsSettings          *TLSSettings
		inputValues          helm.Values
		expectExtraArgsCount int
		expectArgContains    string
	}{
		{
			name: "adds tls-cipher-suites to extraContainerArgs",
			tlsSettings: &TLSSettings{
				CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
			},
			inputValues:          helm.Values{},
			expectExtraArgsCount: 1,
			expectArgContains:    "--tls-cipher-suites=",
		},
		{
			name: "preserves existing extraContainerArgs",
			tlsSettings: &TLSSettings{
				CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			},
			inputValues: helm.Values{
				"pilot": map[string]any{
					"extraContainerArgs": []any{"--some-arg=value"},
				},
			},
			expectExtraArgsCount: 2,
			expectArgContains:    "--tls-cipher-suites=",
		},
		{
			name: "does not add extraContainerArgs when empty settings",
			tlsSettings: &TLSSettings{
				CipherSuites: []string{},
			},
			inputValues:          helm.Values{},
			expectExtraArgsCount: 0,
			expectArgContains:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ApplyAPIServerTLSSettings(tt.tlsSettings, tt.inputValues)
			if err != nil {
				t.Errorf("ApplyAPIServerTLSSettings() error = %v", err)
				return
			}

			args, found, _ := result.GetSlice("pilot.extraContainerArgs")
			if tt.expectExtraArgsCount == 0 {
				if found && len(args) > 0 {
					t.Errorf("Expected no extraContainerArgs, but got %v", args)
				}
				return
			}

			if !found {
				t.Errorf("Expected pilot.extraContainerArgs to be set")
				return
			}

			if len(args) != tt.expectExtraArgsCount {
				t.Errorf("Expected %d extraContainerArgs, got %d: %v", tt.expectExtraArgsCount, len(args), args)
			}

			if tt.expectArgContains != "" {
				found := false
				for _, arg := range args {
					if argStr, ok := arg.(string); ok {
						if len(argStr) >= len(tt.expectArgContains) && argStr[:len(tt.expectArgContains)] == tt.expectArgContains {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("Expected extraContainerArgs to contain %q, got %v", tt.expectArgContains, args)
				}
			}
		})
	}
}
