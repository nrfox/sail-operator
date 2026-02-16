package main

func commonLabels(v *Values) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "sail-operator",
		"app.kubernetes.io/created-by": v.Name,
		"app.kubernetes.io/instance":   v.Deployment.Name,
		"app.kubernetes.io/managed-by": "helm",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    v.Name,
		"control-plane":                v.Deployment.Name,
	}
}

func selectorLabels(v *Values) map[string]string {
	return map[string]string{
		"app.kubernetes.io/created-by": v.Name,
		"app.kubernetes.io/part-of":    v.Name,
		"control-plane":                v.Deployment.Name,
	}
}

func rbacLabels(component, instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "clusterrolebinding",
		"app.kubernetes.io/instance":   instance,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/managed-by": "helm",
	}
}

func metricsReaderLabels(v *Values) map[string]string {
	return map[string]string{
		"app.kubernetes.io/created-by": v.Name,
		"app.kubernetes.io/name":       "clusterrole",
		"app.kubernetes.io/instance":   "metrics-reader",
		"app.kubernetes.io/component":  "kube-rbac-proxy",
		"app.kubernetes.io/managed-by": "helm",
		"app.kubernetes.io/part-of":    v.Name,
	}
}

func ptr[T any](v T) *T {
	return &v
}
