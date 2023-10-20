package management

import (
	"context"
	"encoding/json"

	"github.com/alecthomas/jsonschema"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
)

func (m *Server) GetConfig(
	_ context.Context,
	_ *emptypb.Empty,
) (*managementv1.GatewayConfig, error) {
	lg := m.logger
	objects, err := m.lifecycler.GetObjectList()
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to get object list")
		return nil, err
	}
	gc := &managementv1.GatewayConfig{}
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
		gc.Documents = append(gc.Documents, &managementv1.ConfigDocumentWithSchema{
			Json:   jsonData,
			Yaml:   yamlData,
			Schema: schemaData,
		})
	})
	return gc, nil
}

func (m *Server) UpdateConfig(
	_ context.Context,
	in *managementv1.UpdateConfigRequest,
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
			logger.Err(err),
		).Error("failed to update object list")
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (m *Server) GetDashboardSettings(
	ctx context.Context,
	in *emptypb.Empty,
) (*managementv1.DashboardSettings, error) {
	return m.dashboardSettings.GetDashboardSettings(ctx, in)
}

func (m *Server) UpdateDashboardSettings(
	ctx context.Context,
	in *managementv1.DashboardSettings,
) (*emptypb.Empty, error) {
	return m.dashboardSettings.UpdateDashboardSettings(ctx, in)
}
