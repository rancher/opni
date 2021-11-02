package util

import (
	"context"

	"github.com/minio/minio-go/v7/pkg/credentials"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// S3CredentialsProvider implements the minio credentials.Provider interface
// to retrieve S3 credentials from a Kubernetes secret.
type S3CredentialsProvider struct {
	Client      client.Client
	Namespace   string
	AccessKey   *corev1.SecretKeySelector
	SecretKey   *corev1.SecretKeySelector
	ShouldCache bool // Whether to fetch the credentials from the secret every time.
}

func (p *S3CredentialsProvider) Retrieve() (credentials.Value, error) {
	keysColocated := p.AccessKey.LocalObjectReference == p.SecretKey.LocalObjectReference
	if keysColocated {
		secret := &corev1.Secret{}
		if err := p.Client.Get(context.Background(), client.ObjectKey{
			Namespace: p.Namespace,
			Name:      p.AccessKey.Name,
		}, secret); err != nil {
			return credentials.Value{}, err
		}
		return credentials.Value{
			AccessKeyID:     string(secret.Data[p.AccessKey.Key]),
			SecretAccessKey: string(secret.Data[p.SecretKey.Key]),
		}, nil
	}
	accessKeySecret := &corev1.Secret{}
	if err := p.Client.Get(context.Background(), client.ObjectKey{
		Namespace: p.Namespace,
		Name:      p.AccessKey.Name,
	}, accessKeySecret); err != nil {
		return credentials.Value{}, err
	}
	secretKeySecret := &corev1.Secret{}
	if err := p.Client.Get(context.Background(), client.ObjectKey{
		Namespace: p.Namespace,
		Name:      p.SecretKey.Name,
	}, secretKeySecret); err != nil {
		return credentials.Value{}, err
	}
	return credentials.Value{
		AccessKeyID:     string(accessKeySecret.Data[p.AccessKey.Key]),
		SecretAccessKey: string(secretKeySecret.Data[p.SecretKey.Key]),
	}, nil
}

func (p *S3CredentialsProvider) IsExpired() bool {
	return !p.ShouldCache
}
