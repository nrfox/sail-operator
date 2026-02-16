package main

import corev1 "k8s.io/api/core/v1"

type Values struct {
	Name                     string                                `json:"name"`
	Deployment               DeploymentValues                      `json:"deployment"`
	RevisionHistoryLimit     *int32                                `json:"revisionHistoryLimit,omitempty"`
	Service                  ServiceValues                         `json:"service"`
	ServiceAccountName       string                                `json:"serviceAccountName"`
	OperatorLogLevel         string                                `json:"operatorLogLevel"`
	Image                    string                                `json:"image"`
	ImagePullPolicy          string                                `json:"imagePullPolicy,omitempty"`
	Operator                 OperatorValues                        `json:"operator"`
	BundleGeneration         bool                                  `json:"bundleGeneration,omitempty"`
	Platform                 string                                `json:"platform,omitempty"`
	Pod                      *PodValues                            `json:"pod,omitempty"`
	NodeSelector             map[string]string                     `json:"nodeSelector,omitempty"`
	Tolerations              []corev1.Toleration                   `json:"tolerations,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint    `json:"topologySpreadConstraints,omitempty"`
	PriorityClassName        string                                `json:"priorityClassName,omitempty"`
	ImagePullSecrets         []string                              `json:"imagePullSecrets,omitempty"`
}

type DeploymentValues struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ServiceValues struct {
	Port int32 `json:"port"`
}

type OperatorValues struct {
	Resources ResourceRequirements `json:"resources"`
	Env       []corev1.EnvVar      `json:"env,omitempty"`
	ExtraArgs []string             `json:"extraArgs,omitempty"`
}

type ResourceRequirements struct {
	Limits   ResourceList `json:"limits"`
	Requests ResourceList `json:"requests"`
}

type ResourceList struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type PodValues struct {
	Labels map[string]string `json:"labels,omitempty"`
}

type ReleaseMeta struct {
	Name      string
	Namespace string
}
