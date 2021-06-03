package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	appLabels = map[string]string{
		"app": "drain-service",
	}
	drainServiceSecret = v1.LocalObjectReference{
		Name: "drain-service-config",
	}
	s3Secret = v1.LocalObjectReference{
		Name: "s3-config",
	}
	esSecret = v1.LocalObjectReference{
		Name: "es-config",
	}

	DrainServiceTemplate = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "drain-service",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: appLabels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: appLabels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "drain-service",
							Env: []v1.EnvVar{
								{
									Name: "NATS_SERVER_URL",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: drainServiceSecret,
											Key:                  "nats-server-url",
										},
									},
								},
								{
									Name: "S3_ACCESS_KEY",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: s3Secret,
											Key:                  "access-key",
										},
									},
								},
								{
									Name: "S3_SECRET_KEY",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: s3Secret,
											Key:                  "secret-key",
										},
									},
								},
								{
									Name: "ES_USERNAME",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: esSecret,
											Key:                  "username",
										},
									},
								},
								{
									Name: "ES_PASSWORD",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: esSecret,
											Key:                  "password",
										},
									},
								},
								{
									Name:  "FAIL_KEYWORDS",
									Value: "fail,error,missing,unable",
								},
							},
						},
					},
				},
			},
		},
	}
)
