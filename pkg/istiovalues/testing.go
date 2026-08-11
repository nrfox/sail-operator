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

// testingT is a minimal interface that supports both testing.TB and ginkgo.FullGinkgoTInterface.
// This allows EnableFIPS to work with both standard Go tests and Ginkgo tests.
type testingT interface {
	Helper()
	Cleanup(func())
}

// EnableFIPS overrides fipsEnabled to return true for the duration of the test.
// This should ONLY be used for testing as it always returns true.
func EnableFIPS(t testingT) {
	t.Helper()
	original := fipsEnabled
	t.Cleanup(func() { fipsEnabled = original })
	fipsEnabled = func() bool { return true }
}
