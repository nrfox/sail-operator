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

// This file contains utilities for testing.
package istiovalues

import (
	"testing"
)

// testHelper provides a minimal interface for test cleanup that works with both
// testing.TB and Ginkgo's FullGinkgoTInterface. This avoids issues with Go 1.26's
// addition of ArtifactDir() to testing.TB, which Ginkgo doesn't yet implement.
type testHelper interface {
	Helper()
	Cleanup(func())
}

// EnableFIPS overrides fipsEnabled to return true for the duration of the test.
// This should ONLY be used for testing as it always returns true.
func EnableFIPS(t testing.TB) {
	t.Helper()
	original := fipsEnabled
	t.Cleanup(func() { fipsEnabled = original })
	fipsEnabled = func() bool { return true }
}

// EnableFIPSForGinkgo overrides fipsEnabled to return true for the duration of the test.
// This variant accepts any test helper that supports Helper() and Cleanup(),
// making it compatible with both testing.TB and Ginkgo's GinkgoT().
func EnableFIPSForGinkgo(t testHelper) {
	t.Helper()
	original := fipsEnabled
	t.Cleanup(func() { fipsEnabled = original })
	fipsEnabled = func() bool { return true }
}
