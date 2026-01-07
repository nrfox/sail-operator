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
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/crypto"

	"github.com/istio-ecosystem/sail-operator/pkg/helm"
)

// TLSSettings represents the TLS settings from the OpenShift APIServer.
type TLSSettings struct {
	// CipherSuites is a list of cipher suites.
	CipherSuites []string
}

// GetTLSSettingsFromAPIServer extracts TLS settings from an OpenShift APIServer resource.
func GetTLSSettingsFromAPIServer(apiServer *configv1.APIServer) TLSSettings {
	profile := apiServer.Spec.TLSSecurityProfile
	profileType := configv1.TLSProfileIntermediateType
	if profile != nil {
		profileType = profile.Type
	}

	var profileSpec *configv1.TLSProfileSpec
	if profileType == configv1.TLSProfileCustomType {
		if profile.Custom != nil {
			profileSpec = &profile.Custom.TLSProfileSpec
		}
	} else {
		profileSpec = configv1.TLSProfiles[profileType]
	}

	if profileSpec == nil {
		profileSpec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	return TLSSettings{
		CipherSuites: crypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers),
	}
}

// ApplyAPIServerTLSSettings applies TLS settings from the OpenShift APIServer to the Helm values.
// The TLS settings are applied to:
// - meshConfig.meshMTLS and meshConfig.tlsDefaults (for mesh traffic TLS)
// - pilot.extraContainerArgs with --tls-cipher-suites (for istiod's own TLS server)
func ApplyAPIServerTLSSettings(tlsSettings *TLSSettings, values helm.Values) (helm.Values, error) {
	if tlsSettings == nil {
		return values, nil
	}

	if len(tlsSettings.CipherSuites) > 0 {
		cipherSlice := make([]any, len(tlsSettings.CipherSuites))
		for i, c := range tlsSettings.CipherSuites {
			cipherSlice[i] = c
		}
		if err := values.SetIfAbsent("meshConfig.tlsDefaults.cipherSuites", cipherSlice); err != nil {
			return nil, fmt.Errorf("failed to set meshConfig.tlsDefaults.cipherSuites: %w", err)
		}
		if err := values.SetIfAbsent("meshConfig.meshMTLS.cipherSuites", cipherSlice); err != nil {
			return nil, fmt.Errorf("failed to set meshConfig.meshMTLS.cipherSuites: %w", err)
		}

		// Also set --tls-cipher-suites flag for istiod's own TLS server
		// This controls the cipher suites for istiod's gRPC and HTTPS endpoints
		// See: https://github.com/istio/istio/blob/master/pilot/cmd/pilot-discovery/app/cmd.go
		if err := addExtraContainerArg(values, "--tls-cipher-suites", strings.Join(tlsSettings.CipherSuites, ",")); err != nil {
			return nil, fmt.Errorf("failed to set pilot.extraContainerArgs: %w", err)
		}
	}

	return values, nil
}

// addExtraContainerArg adds an argument to pilot.extraContainerArgs if not already present.
func addExtraContainerArg(values helm.Values, argName, argValue string) error {
	existingArgs, _, _ := values.GetSlice("pilot.extraContainerArgs")

	argWithValue := argName + "=" + argValue
	for _, arg := range existingArgs {
		if argStr, ok := arg.(string); ok {
			// Skip if already set (don't override user-provided values)
			if strings.HasPrefix(argStr, argName) {
				return nil
			}
		}
	}

	// Add the new argument
	newArgs := append(existingArgs, argWithValue)
	return values.Set("pilot.extraContainerArgs", newArgs)
}
