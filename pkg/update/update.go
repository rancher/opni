package update

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/urn"
	"google.golang.org/grpc"
)

var (
	agentSyncHandlerBuilders  = map[string]SyncHandlerBuilder{}
	agentSyncMu               = sync.Mutex{}
	pluginSyncHandlerBuilders = map[string]SyncHandlerBuilder{}
	pluginSyncMu              = sync.Mutex{}
)

type UpdateTypeHandler interface {
	CalculateUpdate(context.Context, *controlv1.UpdateManifest) (*controlv1.PatchList, error)
	CalculateExpectedManifest(context.Context, urn.UpdateType) (*controlv1.UpdateManifest, error)
	Collectors() []prometheus.Collector
	Strategy() string
}

type UpdateStreamInterceptor interface {
	StreamServerInterceptor() grpc.StreamServerInterceptor
}

type SyncHandler interface {
	GetCurrentManifest(context.Context) (*controlv1.UpdateManifest, error)
	HandleSyncResults(context.Context, *controlv1.SyncResults) error
	Strategy() string
}

type ManifestEntryList []*controlv1.UpdateManifestEntry

type SyncHandlerBuilder func(...any) (SyncHandler, error)

func RegisterAgentSyncHandlerBuilder(strategy string, builder SyncHandlerBuilder) {
	agentSyncMu.Lock()
	defer agentSyncMu.Unlock()
	agentSyncHandlerBuilders[strategy] = builder
}

func RegisterPluginSyncHandlerBuilder(strategy string, builder SyncHandlerBuilder) {
	pluginSyncMu.Lock()
	defer pluginSyncMu.Unlock()
	pluginSyncHandlerBuilders[strategy] = builder
}

func GetAgentSyncHandlerBuilder[T ~string](strategy T) SyncHandlerBuilder {
	agentSyncMu.Lock()
	defer agentSyncMu.Unlock()
	return agentSyncHandlerBuilders[string(strategy)]
}

func GetPluginSyncHandlerBuilder[T ~string](strategy T) SyncHandlerBuilder {
	pluginSyncMu.Lock()
	defer pluginSyncMu.Unlock()
	return pluginSyncHandlerBuilders[string(strategy)]
}

type entry interface {
	GetPackage() string
}

func GetType[T entry](entries []T) (urn.UpdateType, error) {
	var updateType urn.UpdateType
	if len(entries) == 0 {
		return "", ErrNoEntries
	}
	for _, entry := range entries {
		opniURN, err := urn.ParseString(entry.GetPackage())
		if err != nil {
			return "", err
		}
		//Validate the URN
		switch {
		case opniURN.Type == urn.Plugin:
			// If update type is not empty or plugin, return error
			switch updateType {
			case "":
				updateType = urn.Plugin
			case urn.Plugin:
			default:
				return "", ErrMultipleTypes
			}
		case opniURN.Type == urn.Agent:
			// If update type is not empty or agent, return error
			switch updateType {
			case "":
				updateType = urn.Agent
			case urn.Agent:
			default:
				return "", ErrMultipleTypes
			}
		default:
			return "", ErrInvalidType
		}
	}
	return updateType, nil
}

func getStrategy[T entry](entries []T) (string, error) {
	var strategy string
	if len(entries) == 0 {
		return "", ErrNoEntries
	}
	for _, entry := range entries {
		opniURN, err := urn.ParseString(entry.GetPackage())
		if err != nil {
			return "", err
		}
		switch strategy {
		case "":
			strategy = opniURN.Strategy
		case opniURN.Strategy:
		default:
			return "", ErrMultipleStrategies
		}
	}
	return strategy, nil
}
