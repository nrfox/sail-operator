package main

import (
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func buildDeployment(v *Values, meta *ReleaseMeta) *appsv1.Deployment {
	labels := commonLabels(v)
	for k, val := range v.Deployment.Labels {
		labels[k] = val
	}

	annotations := map[string]string{
		"kubectl.kubernetes.io/default-container": "sail-operator",
	}
	for k, val := range v.Deployment.Annotations {
		annotations[k] = val
	}

	podLabels := maps.Clone(selectorLabels(v))
	if v.Pod != nil {
		for k, val := range v.Pod.Labels {
			podLabels[k] = val
		}
	}

	args := []string{
		"--health-probe-bind-address=:8081",
		"--metrics-bind-address=:8443",
		fmt.Sprintf("--zap-log-level=%s", v.OperatorLogLevel),
	}
	args = append(args, v.Operator.ExtraArgs...)

	container := corev1.Container{
		Name:    "sail-operator",
		Command: []string{"/sail-operator"},
		Args:    args,
		Image:   v.Image,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(8081),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(8081),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(v.Operator.Resources.Limits.CPU),
				corev1.ResourceMemory: resource.MustParse(v.Operator.Resources.Limits.Memory),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(v.Operator.Resources.Requests.CPU),
				corev1.ResourceMemory: resource.MustParse(v.Operator.Resources.Requests.Memory),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			ReadOnlyRootFilesystem: ptr(true),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "operator-config",
				MountPath: "/etc/sail-operator",
				ReadOnly:  true,
			},
		},
	}

	if v.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(v.ImagePullPolicy)
	}

	if len(v.Operator.Env) > 0 {
		container.Env = v.Operator.Env
	}

	defaultMode := int32(420)
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Deployment.Name,
			Namespace: meta.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             ptr(int32(1)),
			RevisionHistoryLimit: v.RevisionHistoryLimit,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels(v),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      podLabels,
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/arch",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"amd64", "arm64", "ppc64le", "s390x"},
											},
											{
												Key:      "kubernetes.io/os",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"linux"},
											},
										},
									},
								},
							},
						},
					},
					Containers:                    []corev1.Container{container},
					SecurityContext:               &corev1.PodSecurityContext{RunAsNonRoot: ptr(true)},
					ServiceAccountName:            v.ServiceAccountName,
					TerminationGracePeriodSeconds: ptr(int64(10)),
					Volumes: []corev1.Volume{
						{
							Name: "operator-config",
							VolumeSource: corev1.VolumeSource{
								DownwardAPI: &corev1.DownwardAPIVolumeSource{
									DefaultMode: &defaultMode,
									Items: []corev1.DownwardAPIVolumeFile{
										{
											Path: "config.properties",
											FieldRef: &corev1.ObjectFieldSelector{
												FieldPath: "metadata.annotations",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if len(v.NodeSelector) > 0 {
		dep.Spec.Template.Spec.NodeSelector = v.NodeSelector
	}
	if v.PriorityClassName != "" {
		dep.Spec.Template.Spec.PriorityClassName = v.PriorityClassName
	}
	if len(v.Tolerations) > 0 {
		dep.Spec.Template.Spec.Tolerations = v.Tolerations
	}
	if len(v.TopologySpreadConstraints) > 0 {
		dep.Spec.Template.Spec.TopologySpreadConstraints = v.TopologySpreadConstraints
	}

	return dep
}
