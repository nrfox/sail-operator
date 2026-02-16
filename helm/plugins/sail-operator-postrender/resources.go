package main

import (
	"bytes"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func ParseValuesFromManifests(manifests []byte) (*Values, *ReleaseMeta, error) {
	docs := bytes.Split(manifests, []byte("\n---"))

	for _, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		var cm corev1.ConfigMap
		if err := yaml.Unmarshal(doc, &cm); err != nil {
			continue
		}

		if cm.Name != "sail-operator-values-carrier" {
			continue
		}

		valuesYAML, ok := cm.Data["values.yaml"]
		if !ok {
			return nil, nil, fmt.Errorf("carrier ConfigMap missing values.yaml data key")
		}

		var v Values
		if err := yaml.Unmarshal([]byte(valuesYAML), &v); err != nil {
			return nil, nil, fmt.Errorf("unmarshaling values: %w", err)
		}

		meta := &ReleaseMeta{
			Name:      cm.Annotations["sail-operator/release-name"],
			Namespace: cm.Annotations["sail-operator/release-namespace"],
		}

		return &v, meta, nil
	}

	return nil, nil, fmt.Errorf("carrier ConfigMap not found in manifests")
}

func BuildAllResources(v *Values, meta *ReleaseMeta) ([]byte, error) {
	resources := []interface{}{
		buildServiceAccount(v, meta),
		buildDeployment(v, meta),
		buildService(v, meta),
		buildClusterRole(v),
		buildClusterRoleBinding(v, meta),
		buildLeaderElectionRole(v, meta),
		buildLeaderElectionRoleBinding(v, meta),
		buildProxyClusterRole(v),
		buildProxyClusterRoleBinding(v, meta),
		buildMetricsReaderClusterRole(v),
	}

	var buf bytes.Buffer
	for i, r := range resources {
		if i > 0 {
			buf.WriteString("---\n")
		}
		data, err := yaml.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("marshaling resource %d: %w", i, err)
		}
		buf.Write(data)
	}

	return buf.Bytes(), nil
}
