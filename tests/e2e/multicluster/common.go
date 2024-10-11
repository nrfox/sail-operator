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

package multicluster

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/istio-ecosystem/sail-operator/tests/e2e/util/kubectl"
	. "github.com/onsi/gomega"
)

// verifyResponsesAreReceivedFromBothClusters checks that when the sleep pod in the sample namespace
// sends a request to the helloworld service, it receives responses from both v1 and v2 versions,
// which are deployed in different clusters
func verifyResponsesAreReceivedFromBothClusters(k kubectl.Kubectl, clusterName string, expectedVersions ...string) {
	if len(expectedVersions) == 0 {
		expectedVersions = []string{"v1", "v2"}
	}
	for _, v := range expectedVersions {
		Eventually(k.WithNamespace("sample").Exec, 10*time.Second, 10*time.Millisecond).
			WithArguments("deploy/sleep", "sleep", "curl -sS helloworld.sample:5000/hello").
			Should(ContainSubstring(fmt.Sprintf("Hello version: %s", v)),
				fmt.Sprintf("sleep pod in %s did not receive any response from %s", clusterName, v))
	}
}

// genTemplate takes a YAML string with go template annotations and a structure
// that fills in those annotations and outputs a YAML string with that structure
// applied to the template.
// Example: version: {{ .Version }} | {Version: "1.2.3"} --> version: Version: "1.2.3"
// Any errors will fail the test.
func genTemplate(manifestTmpl string, values any) string {
	tmpl, err := template.New("manifest-template").Parse(manifestTmpl)
	Expect(err).ShouldNot(HaveOccurred(),
		"template is likely either malformed YAML or the values do not match what is expected")

	var b strings.Builder
	Expect(tmpl.Execute(&b, values)).To(Succeed())
	return b.String()
}
