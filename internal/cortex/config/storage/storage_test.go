package storage_test

import (
	"testing"
	time "time"

	"github.com/cortexproject/cortex/pkg/storage/bucket/http"
	"github.com/cortexproject/cortex/pkg/storage/bucket/s3"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/rancher/opni/internal/cortex/config/storage"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v2"
	k8syaml "sigs.k8s.io/yaml"
)

func TestMarshal(t *testing.T) {
	original := &s3.Config{
		Endpoint:         "http://localhost:9000",
		Region:           "asdf",
		BucketName:       "asdf",
		SecretAccessKey:  flagext.Secret{Value: "asdf"},
		AccessKeyID:      "asdf",
		Insecure:         false,
		SignatureVersion: "asdf",
		BucketLookupType: "asdf",
		SSE: s3.SSEConfig{
			Type:                 "asdf",
			KMSKeyID:             "asdf",
			KMSEncryptionContext: "asdf",
		},
		HTTP: s3.HTTPConfig{
			Config: http.Config{
				ResponseHeaderTimeout: 1 * time.Second,
				InsecureSkipVerify:    true,
				TLSHandshakeTimeout:   1 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       1 * time.Second,
			},
		},
	}
	proto := &storage.S3Config{
		Endpoint:         lo.ToPtr("http://localhost:9000"),
		Region:           lo.ToPtr("asdf"),
		BucketName:       lo.ToPtr("asdf"),
		SecretAccessKey:  lo.ToPtr("asdf"),
		AccessKeyId:      lo.ToPtr("asdf"),
		Insecure:         lo.ToPtr(false),
		SignatureVersion: lo.ToPtr("asdf"),
		BucketLookupType: lo.ToPtr("asdf"),
		Sse: &storage.S3SSEConfig{
			Type:                 lo.ToPtr("asdf"),
			KmsKeyId:             lo.ToPtr("asdf"),
			KmsEncryptionContext: lo.ToPtr("asdf"),
		},
		Http: &storage.HttpConfig{
			IdleConnTimeout:           durationpb.New(1 * time.Second),
			ResponseHeaderTimeout:     durationpb.New(1 * time.Second),
			InsecureSkipVerify:        lo.ToPtr(true),
			TlsHandshakeTimeout:       durationpb.New(1 * time.Second),
			ExpectContinueTimeout:     durationpb.New(1 * time.Second),
			MaxIdleConnections:        lo.ToPtr[int32](1),
			MaxIdleConnectionsPerHost: lo.ToPtr[int32](1),
			MaxConnectionsPerHost:     lo.ToPtr[int32](1),
		},
	}

	upstreamYamlData, err := yaml.Marshal(original)

	jsonData, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		UseProtoNames:   true,
		EmitUnpopulated: true,
	}.Marshal(proto)
	if err != nil {
		t.Fatal(err)
	}
	yamlData, _ := k8syaml.JSONToYAML(jsonData)
	t.Logf("\nupstream: \n%s\n", string(upstreamYamlData))
	t.Logf("\nproto: \n%s\n", string(yamlData))
}
