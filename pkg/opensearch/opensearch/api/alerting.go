package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type AlertingAPI struct {
	MonitorAPI
	DestinationAPI
	AlertAPI
}

type MonitorAPI struct {
	*opensearchtransport.Client
}

type DestinationAPI struct {
	*opensearchtransport.Client
}

type AlertAPI struct {
	*opensearchtransport.Client
}

func generateMonitorBase() strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_alerting") + 1 + len("monitors"))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_alerting")
	path.WriteString("/")
	path.WriteString("monitors")
	return path
}

func getMonitorPath(monitorId string) strings.Builder {
	path := generateMonitorBase()
	path.Grow(1 + len(monitorId))
	path.WriteString("/")
	path.WriteString(monitorId)
	return path
}

func (m *MonitorAPI) CreateMonitor(ctx context.Context, body io.Reader) (*Response, error) {
	path := generateMonitorBase()

	req, err := http.NewRequest(http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) GetMonitor(ctx context.Context, monitorId string) (*Response, error) {
	path := getMonitorPath(monitorId)

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) UpdateMonitor(ctx context.Context, monitorId string, body io.Reader) (*Response, error) {
	path := getMonitorPath(monitorId)

	req, err := http.NewRequest(http.MethodPut, path.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) DeleteMonitor(ctx context.Context, monitorId string) (*Response, error) {
	path := getMonitorPath(monitorId)

	req, err := http.NewRequest(http.MethodDelete, path.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func generateDestinationBase() strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_alerting") + 1 + len("destinations"))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_alerting")
	path.WriteString("/")
	path.WriteString("destinations")
	return path
}

func getDestinationBase(destinationId string) strings.Builder {
	path := generateDestinationBase()
	path.Grow(1 + len(destinationId))
	path.WriteString("/")
	path.WriteString(destinationId)
	return path
}

func (d *DestinationAPI) CreateDestination(ctx context.Context, body io.Reader) (*Response, error) {
	path := generateDestinationBase()

	req, err := http.NewRequest(http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *DestinationAPI) GetDestination(ctx context.Context, destinationId string) (*Response, error) {
	path := getDestinationBase(destinationId)

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *DestinationAPI) ListDestinations(ctx context.Context) (*Response, error) {
	path := generateDestinationBase()

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *DestinationAPI) UpdateDestination(ctx context.Context, destinationId string, body io.Reader) (*Response, error) {
	path := getDestinationBase(destinationId)

	req, err := http.NewRequest(http.MethodPut, path.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *DestinationAPI) DeleteDestination(ctx context.Context, destinationId string) (*Response, error) {
	path := getDestinationBase(destinationId)

	req, err := http.NewRequest(http.MethodDelete, path.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (a *AlertAPI) ListAlerts(ctx context.Context) (*Response, error) {
	path := generateMonitorBase()
	path.Grow(1 + len("alerts"))
	path.WriteString("/")
	path.WriteString("alerts")

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *AlertAPI) AcknowledgeAlert(ctx context.Context, monitorId string, body io.Reader) (*Response, error) {
	path := getMonitorPath(monitorId)
	path.Grow(1 + len("_acknowledge") + 1 + len("alerts"))
	path.WriteString("/")
	path.WriteString("_acknowledge")
	path.WriteString("/")
	path.WriteString("alerts")

	req, err := http.NewRequest(http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}
