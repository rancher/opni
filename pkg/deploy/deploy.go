package deploy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"strings"
	"time"

	"github.com/rancher/wrangler/pkg/objectset"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	InfraStack       = "infra-stack"
	OpniStack        = "opni-stack"
	ServicesStack    = "services"
	OpniConfig       = "opni-config"
	OpniSystemNS     = "opni-system"
	MINIO_ACCESS_KEY = "MINIO_ACCESS_KEY"
	MINIO_SECRET_KEY = "MINIO_SECRET_KEY"
	MINIO_VERSION    = "MINIO_VERSION"
	NATS_VERSION     = "NATS_VERSION"
	NATS_PASSWORD    = "NATS_PASSWORD"
	NATS_REPLICAS    = "NATS_REPLICAS"
	NATS_MAX_PAYLOAD = "NATS_MAX_PAYLOAD"
	NVIDIA_VERSION   = "NVIDIA_VERSION"
	TRAEFIK_VERSION  = "TRAEFIK_VERSION"
	ES_USER          = "ES_USER"
	ES_PASSWORD      = "ES_PASSWORD"
	waitTime         = 1 * time.Minute
)

func Install(ctx context.Context, sc *Context, values map[string]string, disabledItems []string) error {
	// installing infra resources
	logrus.Infof("Deploying infrastructure resources")
	infraObjs, infraOwner, err := objs(InfraStack, values, disabledItems)
	if err != nil {
		return err
	}
	os := objectset.NewObjectSet()
	os.Add(infraObjs...)
	if err := sc.Apply.WithOwner(infraOwner).WithSetID(InfraStack).Apply(os); err != nil {
		return err
	}

	// wait for ns creation
	waitForNS(ctx, sc, OpniSystemNS)

	// initialize configuration secrets
	logrus.Infof("Initializing infrastructure configuration")
	configObj, configOwner := configObj(values)
	os = objectset.NewObjectSet()
	os.Add(configObj...)
	if err := sc.Apply.WithOwner(configOwner).WithSetID(OpniConfig).Apply(os); err != nil {
		return err
	}

	// installing opni stack
	logrus.Infof("Deploying opni stack")
	opniObjs, opniOwner, err := objs(OpniStack, values, disabledItems)
	if err != nil {
		return err
	}

	os = objectset.NewObjectSet()
	os.Add(opniObjs...)
	if err := sc.Apply.WithOwner(opniOwner).WithSetID(OpniStack).Apply(os); err != nil {
		return err
	}

	// wait time before running opni stack
	waitForOpniStack(ctx, sc)

	// installing services stack
	logrus.Infof("Deploying services stack")
	servicesObj, servicesOwner, err := objs(ServicesStack, values, disabledItems)
	if err != nil {
		return err
	}

	os = objectset.NewObjectSet()
	os.Add(servicesObj...)
	return sc.Apply.WithOwner(servicesOwner).WithSetID(ServicesStack).Apply(os)
}

func objs(dir string, values map[string]string, disabledItems []string) ([]runtime.Object, *corev1.ConfigMap, error) {
	owner := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dir,
			Namespace: OpniSystemNS,
		},
	}
	objs := []runtime.Object{}
	for _, asset := range AssetNames() {
		if !strings.HasPrefix(asset, dir) {
			continue
		}
		if disabled(asset, disabledItems) {
			logrus.Infof("%s is disabled", asset)
			continue
		}
		content, err := getManifest(asset)
		if err != nil {
			return nil, nil, err
		}
		newContent := replaceValues(content, values)
		assetObj, err := yamlToObjects(bytes.NewBuffer(newContent))
		if err != nil {
			return nil, nil, err
		}
		objs = append(objs, assetObj...)
	}
	objs = append(objs, owner)
	return objs, owner, nil
}

func yamlToObjects(in io.Reader) ([]runtime.Object, error) {
	var result []runtime.Object
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(in, 4096))
	for {
		raw, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		obj, err := toObjects(raw)
		if err != nil {
			return nil, err
		}

		result = append(result, obj...)
	}

	return result, nil
}

func toObjects(bytes []byte) ([]runtime.Object, error) {
	bytes, err := yamlDecoder.ToJSON(bytes)
	if err != nil {
		return nil, err
	}

	obj, _, err := unstructured.UnstructuredJSONScheme.Decode(bytes, nil, nil)
	if err != nil {
		return nil, err
	}

	if l, ok := obj.(*unstructured.UnstructuredList); ok {
		var result []runtime.Object
		for _, obj := range l.Items {
			copy := obj
			result = append(result, &copy)
		}
		return result, nil
	}

	return []runtime.Object{obj}, nil
}

func getManifest(name string, args ...string) ([]byte, error) {
	asset, err := Asset(name)
	if err != nil {
		return nil, err
	}
	for _, arg := range args {
		kv := strings.Split(arg, "=")
		asset = []byte(strings.Replace(string(asset), "%"+kv[0]+"%", kv[1], 1))
	}
	return asset, nil
}

func replaceValues(content []byte, values map[string]string) []byte {
	contentStr := string(content)
	for k, v := range values {
		contentStr = strings.Replace(contentStr, "%"+k+"%", v, -1)
	}
	return []byte(contentStr)
}

func base64Encode(value string) []byte {
	return []byte(base64.StdEncoding.EncodeToString([]byte(value)))
}

func configObj(values map[string]string) ([]runtime.Object, *corev1.Secret) {
	cfgSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniConfig,
			Namespace: OpniSystemNS,
		},
		Data: map[string][]byte{
			MINIO_ACCESS_KEY: []byte(values[MINIO_ACCESS_KEY]),
			MINIO_SECRET_KEY: []byte(values[MINIO_ACCESS_KEY]),
			NATS_PASSWORD:    []byte(values[NATS_PASSWORD]),
			ES_PASSWORD:      []byte(values[ES_PASSWORD]),
		},
	}
	return []runtime.Object{cfgSecret}, cfgSecret
}

func waitForOpniStack(ctx context.Context, sc *Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		esStatefulSet, err := sc.K8s.AppsV1().StatefulSets(OpniSystemNS).Get(ctx, "opendistro-es-master", metav1.GetOptions{})
		if err != nil {
			logrus.Infof("Waiting for elastic search to start correctly")
			continue
		}
		if esStatefulSet.Status.ReadyReplicas == *esStatefulSet.Spec.Replicas {
			logrus.Infof("Opni stack is ready")
			break
		}
	}
}

func disabled(asset string, disabledItems []string) bool {
	for _, disabled := range disabledItems {
		if strings.Contains(asset, disabled) {
			return true
		}
	}
	return false
}

func waitForNS(ctx context.Context, sc *Context, ns string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		_, err := sc.K8s.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if err != nil {
			logrus.Infof("Waiting for namespace %s creation", OpniSystemNS)
			continue
		}
		logrus.Infof("%s namespace is ready", OpniSystemNS)
		break
	}
}
