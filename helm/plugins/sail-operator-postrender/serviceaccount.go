package main

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildServiceAccount(v *Values, meta *ReleaseMeta) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.ServiceAccountName,
			Namespace: meta.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "serviceaccount",
				"app.kubernetes.io/instance":   v.ServiceAccountName,
				"app.kubernetes.io/component":  "rbac",
				"app.kubernetes.io/managed-by": "helm",
			},
		},
	}

	for _, s := range v.ImagePullSecrets {
		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: s})
	}

	return sa
}
