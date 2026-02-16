//go:build !wasip1

package main

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func carrierConfigMap(namespace, valuesYAML string) string {
	return `apiVersion: v1
kind: ConfigMap
metadata:
  name: sail-operator-values-carrier
  namespace: ` + namespace + `
  annotations:
    sail-operator/release-name: sail-operator
    sail-operator/release-namespace: ` + namespace + `
data:
  values.yaml: |
` + indent(valuesYAML, 4)
}

func indent(s string, n int) string {
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = prefix + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func parseYAMLDocuments(t *testing.T, data []byte) []map[string]interface{} {
	t.Helper()
	docs := strings.Split(string(data), "---")
	var results []map[string]interface{}
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			t.Fatalf("unmarshal YAML doc: %v\nDoc:\n%s", err, doc)
		}
		results = append(results, obj)
	}
	return results
}

func findResource(docs []map[string]interface{}, kind string) map[string]interface{} {
	for _, doc := range docs {
		if doc["kind"] == kind {
			return doc
		}
	}
	return nil
}

func findResourceByKindAndName(docs []map[string]interface{}, kind, name string) map[string]interface{} {
	for _, doc := range docs {
		if doc["kind"] != kind {
			continue
		}
		meta, ok := doc["metadata"].(map[string]interface{})
		if !ok {
			continue
		}
		if meta["name"] == name {
			return doc
		}
	}
	return nil
}

const defaultValues = `name: sailoperator
deployment:
  name: sail-operator
  annotations:
    images.v1_28_3.istiod: gcr.io/istio-release/pilot:1.28.3
revisionHistoryLimit: 10
service:
  port: 8443
serviceAccountName: sail-operator
operatorLogLevel: info
image: quay.io/sail-dev/sail-operator:1.29-latest
operator:
  resources:
    limits:
      cpu: 500m
      memory: 1024Mi
    requests:
      cpu: 10m
      memory: 64Mi
platform: kubernetes
`

func TestParseValuesFromManifests(t *testing.T) {
	manifests := carrierConfigMap("sail-operator-system", defaultValues)

	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	if v.Name != "sailoperator" {
		t.Errorf("Name = %q, want %q", v.Name, "sailoperator")
	}
	if v.Deployment.Name != "sail-operator" {
		t.Errorf("Deployment.Name = %q, want %q", v.Deployment.Name, "sail-operator")
	}
	if meta.Namespace != "sail-operator-system" {
		t.Errorf("Namespace = %q, want %q", meta.Namespace, "sail-operator-system")
	}
	if meta.Name != "sail-operator" {
		t.Errorf("ReleaseName = %q, want %q", meta.Name, "sail-operator")
	}
}

func TestBuildAllResources(t *testing.T) {
	manifests := carrierConfigMap("sail-operator-system", defaultValues)
	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	output, err := BuildAllResources(v, meta)
	if err != nil {
		t.Fatalf("BuildAllResources failed: %v", err)
	}

	docs := parseYAMLDocuments(t, output)
	if len(docs) != 10 {
		t.Fatalf("expected 10 resources, got %d", len(docs))
	}

	sa := findResource(docs, "ServiceAccount")
	if sa == nil {
		t.Fatal("ServiceAccount not found")
	}
	m := sa["metadata"].(map[string]interface{})
	if m["name"] != "sail-operator" {
		t.Errorf("SA name = %v, want sail-operator", m["name"])
	}

	dep := findResource(docs, "Deployment")
	if dep == nil {
		t.Fatal("Deployment not found")
	}
	depMeta := dep["metadata"].(map[string]interface{})
	if depMeta["name"] != "sail-operator" {
		t.Errorf("Deployment name = %v, want sail-operator", depMeta["name"])
	}
	if depMeta["namespace"] != "sail-operator-system" {
		t.Errorf("Deployment namespace = %v, want sail-operator-system", depMeta["namespace"])
	}

	svc := findResource(docs, "Service")
	if svc == nil {
		t.Fatal("Service not found")
	}
	svcMeta := svc["metadata"].(map[string]interface{})
	if svcMeta["name"] != "sail-operator-metrics-service" {
		t.Errorf("Service name = %v, want sail-operator-metrics-service", svcMeta["name"])
	}

	cr := findResourceByKindAndName(docs, "ClusterRole", "sailoperator-role")
	if cr == nil {
		t.Fatal("ClusterRole sailoperator-role not found")
	}

	crb := findResourceByKindAndName(docs, "ClusterRoleBinding", "sailoperator-rolebinding")
	if crb == nil {
		t.Fatal("ClusterRoleBinding sailoperator-rolebinding not found")
	}

	role := findResourceByKindAndName(docs, "Role", "leader-election-role")
	if role == nil {
		t.Fatal("Role leader-election-role not found")
	}

	rb := findResourceByKindAndName(docs, "RoleBinding", "leader-election-rolebinding")
	if rb == nil {
		t.Fatal("RoleBinding leader-election-rolebinding not found")
	}

	proxyCR := findResourceByKindAndName(docs, "ClusterRole", "sailoperator-proxy-role")
	if proxyCR == nil {
		t.Fatal("ClusterRole sailoperator-proxy-role not found")
	}

	proxyCRB := findResourceByKindAndName(docs, "ClusterRoleBinding", "sailoperator-proxy-rolebinding")
	if proxyCRB == nil {
		t.Fatal("ClusterRoleBinding sailoperator-proxy-rolebinding not found")
	}

	metricsReader := findResourceByKindAndName(docs, "ClusterRole", "metrics-reader")
	if metricsReader == nil {
		t.Fatal("ClusterRole metrics-reader not found")
	}
}

func TestCustomNodeSelector(t *testing.T) {
	values := defaultValues + `nodeSelector:
  disktype: ssd
`
	manifests := carrierConfigMap("default", values)
	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	output, err := BuildAllResources(v, meta)
	if err != nil {
		t.Fatalf("BuildAllResources failed: %v", err)
	}

	docs := parseYAMLDocuments(t, output)
	dep := findResource(docs, "Deployment")
	if dep == nil {
		t.Fatal("Deployment not found")
	}

	spec := dep["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	ns := podSpec["nodeSelector"].(map[string]interface{})
	if ns["disktype"] != "ssd" {
		t.Errorf("nodeSelector disktype = %v, want ssd", ns["disktype"])
	}
}

func TestExtraArgs(t *testing.T) {
	values := `name: sailoperator
deployment:
  name: sail-operator
  annotations: {}
revisionHistoryLimit: 10
service:
  port: 8443
serviceAccountName: sail-operator
operatorLogLevel: info
image: quay.io/sail-dev/sail-operator:1.29-latest
operator:
  resources:
    limits:
      cpu: 500m
      memory: 1024Mi
    requests:
      cpu: 10m
      memory: 64Mi
  extraArgs:
    - --foo=bar
    - --baz=qux
platform: kubernetes
`
	manifests := carrierConfigMap("default", values)
	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	output, err := BuildAllResources(v, meta)
	if err != nil {
		t.Fatalf("BuildAllResources failed: %v", err)
	}

	docs := parseYAMLDocuments(t, output)
	dep := findResource(docs, "Deployment")
	if dep == nil {
		t.Fatal("Deployment not found")
	}

	spec := dep["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	args := container["args"].([]interface{})

	found := false
	for _, a := range args {
		if a == "--foo=bar" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --foo=bar in args, got %v", args)
	}
}

func TestImagePullSecrets(t *testing.T) {
	values := defaultValues + `imagePullSecrets:
  - my-registry-secret
`
	manifests := carrierConfigMap("default", values)
	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	output, err := BuildAllResources(v, meta)
	if err != nil {
		t.Fatalf("BuildAllResources failed: %v", err)
	}

	docs := parseYAMLDocuments(t, output)
	sa := findResource(docs, "ServiceAccount")
	if sa == nil {
		t.Fatal("ServiceAccount not found")
	}

	secrets := sa["imagePullSecrets"].([]interface{})
	if len(secrets) != 1 {
		t.Fatalf("expected 1 imagePullSecret, got %d", len(secrets))
	}
	secret := secrets[0].(map[string]interface{})
	if secret["name"] != "my-registry-secret" {
		t.Errorf("imagePullSecret name = %v, want my-registry-secret", secret["name"])
	}
}

func TestTolerations(t *testing.T) {
	values := defaultValues + `tolerations:
  - key: "node-role.kubernetes.io/master"
    operator: "Exists"
    effect: "NoSchedule"
`
	manifests := carrierConfigMap("default", values)
	v, meta, err := ParseValuesFromManifests([]byte(manifests))
	if err != nil {
		t.Fatalf("ParseValuesFromManifests failed: %v", err)
	}

	output, err := BuildAllResources(v, meta)
	if err != nil {
		t.Fatalf("BuildAllResources failed: %v", err)
	}

	docs := parseYAMLDocuments(t, output)
	dep := findResource(docs, "Deployment")
	if dep == nil {
		t.Fatal("Deployment not found")
	}

	spec := dep["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	tolerations := podSpec["tolerations"].([]interface{})
	if len(tolerations) != 1 {
		t.Fatalf("expected 1 toleration, got %d", len(tolerations))
	}
	tol := tolerations[0].(map[string]interface{})
	if tol["key"] != "node-role.kubernetes.io/master" {
		t.Errorf("toleration key = %v, want node-role.kubernetes.io/master", tol["key"])
	}
}
