package main

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func buildService(v *Values, meta *ReleaseMeta) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Deployment.Name + "-metrics-service",
			Namespace: meta.Namespace,
			Labels:    commonLabels(v),
		},
		Spec: corev1.ServiceSpec{
			IPFamilyPolicy: ptr(corev1.IPFamilyPolicyPreferDualStack),
			Ports: []corev1.ServicePort{
				{
					Name:     "https",
					Port:     v.Service.Port,
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.FromInt32(v.Service.Port),
				},
			},
			Selector: map[string]string{
				"control-plane": v.Deployment.Name,
			},
		},
	}
}
