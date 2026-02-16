package main

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildClusterRole(v *Values) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: v.Name + "-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{
					"configmaps", "endpoints", "events", "namespaces", "nodes",
					"persistentvolumeclaims", "pods", "replicationcontrollers",
					"resourcequotas", "secrets", "serviceaccounts", "services",
				},
				Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"admissionregistration.k8s.io"},
				Resources: []string{
					"mutatingwebhookconfigurations", "validatingadmissionpolicies",
					"validatingadmissionpolicybindings", "validatingwebhookconfigurations",
				},
				Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"apiextensions.k8s.io"},
				Resources: []string{"customresourcedefinitions"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"daemonsets", "deployments"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"autoscaling"},
				Resources: []string{"horizontalpodautoscalers"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"k8s.cni.cncf.io"},
				Resources: []string{"network-attachment-definitions"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"networking.istio.io"},
				Resources: []string{"envoyfilters"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"networkpolicies"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisions"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisions/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisions/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisiontags"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisiontags/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiorevisiontags/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiocnis"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiocnis/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istiocnis/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istios"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istios/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"istios/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"remoteistios"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"remoteistios/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"remoteistios/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{"policy"},
				Resources: []string{"poddisruptionbudgets"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"clusterrolebindings", "clusterroles", "rolebindings", "roles"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch", "bind", "escalate"},
			},
			{
				APIGroups:     []string{"security.openshift.io"},
				ResourceNames: []string{"privileged"},
				Resources:     []string{"securitycontextconstraints"},
				Verbs:         []string{"use"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"ztunnels"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"ztunnels/finalizers"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"sailoperator.io"},
				Resources: []string{"ztunnels/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
		},
	}
}

func buildClusterRoleBinding(v *Values, meta *ReleaseMeta) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   v.Name + "-rolebinding",
			Labels: rbacLabels("rbac", v.Name+"-rolebinding"),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     v.Name + "-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      v.Deployment.Name,
				Namespace: meta.Namespace,
			},
		},
	}
}

func buildLeaderElectionRole(v *Values, meta *ReleaseMeta) *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leader-election-role",
			Namespace: meta.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "role",
				"app.kubernetes.io/instance":   "leader-election-role",
				"app.kubernetes.io/component":  "rbac",
				"app.kubernetes.io/managed-by": "helm",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
		},
	}
}

func buildLeaderElectionRoleBinding(v *Values, meta *ReleaseMeta) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leader-election-rolebinding",
			Namespace: meta.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "rolebinding",
				"app.kubernetes.io/instance":   "leader-election-rolebinding",
				"app.kubernetes.io/component":  "rbac",
				"app.kubernetes.io/managed-by": "helm",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "leader-election-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      v.ServiceAccountName,
				Namespace: meta.Namespace,
			},
		},
	}
}

func buildProxyClusterRole(v *Values) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: v.Name + "-proxy-role",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "clusterrole",
				"app.kubernetes.io/instance":   v.Name + "-proxy-role",
				"app.kubernetes.io/component":  "kube-rbac-proxy",
				"app.kubernetes.io/managed-by": "helm",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}
}

func buildProxyClusterRoleBinding(v *Values, meta *ReleaseMeta) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: v.Name + "-proxy-rolebinding",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "clusterrolebinding",
				"app.kubernetes.io/instance":   v.Name + "-proxy-rolebinding",
				"app.kubernetes.io/component":  "kube-rbac-proxy",
				"app.kubernetes.io/managed-by": "helm",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     v.Name + "-proxy-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      v.ServiceAccountName,
				Namespace: meta.Namespace,
			},
		},
	}
}

func buildMetricsReaderClusterRole(v *Values) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "metrics-reader",
			Labels: metricsReaderLabels(v),
		},
		Rules: []rbacv1.PolicyRule{
			{
				NonResourceURLs: []string{"/metrics"},
				Verbs:           []string{"get"},
			},
		},
	}
}
