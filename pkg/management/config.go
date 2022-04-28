package management

import (
	"context"
	"encoding/json"

	"github.com/alecthomas/jsonschema"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/validation"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/yaml"
)

func (m *Server) GetConfig(
	ctx context.Context,
	_ *emptypb.Empty,
) (*GatewayConfig, error) {
	lg := m.logger
	objects, err := m.lifecycler.GetObjectList()
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to get object list")
		return nil, err
	}
	gc := &GatewayConfig{}
	objects.Visit(func(obj interface{}, schema *jsonschema.Schema) {
		jsonData, err := json.Marshal(obj)
		if err != nil {
			return
		}
		yamlData, err := yaml.Marshal(obj)
		if err != nil {
			return
		}
		schemaData, err := schema.MarshalJSON()
		if err != nil {
			return
		}
		gc.Documents = append(gc.Documents, &ConfigDocumentWithSchema{
			Json:   jsonData,
			Yaml:   yamlData,
			Schema: schemaData,
		})
	})
	return gc, nil
}

func (m *Server) UpdateConfig(
	ctx context.Context,
	in *UpdateConfigRequest,
) (*emptypb.Empty, error) {
	lg := m.logger
	if err := validation.Validate(in); err != nil {
		return nil, err
	}

	lg.Info("handling config update request")
	objList := []meta.Object{}
	for _, doc := range in.Documents {
		obj, err := config.LoadObject(doc.Json)
		if err != nil {
			return nil, err
		}
		objList = append(objList, obj)
	}

	if err := m.lifecycler.UpdateObjectList(objList); err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to update object list")
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
