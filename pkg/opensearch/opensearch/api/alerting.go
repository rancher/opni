package api

import (
	"context"
	"io"
	"net/http"
	"path"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

const (
	monitorBase      = "/_plugins/_alerting/monitors"
	notificationBase = "/_plugins/_notifications/configs"
)

type AlertingAPI struct {
	MonitorAPI
	NotificationAPI
	AlertAPI
}

type MonitorAPI struct {
	*opensearchtransport.Client
}

type NotificationAPI struct {
	*opensearchtransport.Client
}

type AlertAPI struct {
	*opensearchtransport.Client
}

func getMonitorPath(monitorId string) string {
	return path.Join(monitorBase, monitorId)
}

func (m *MonitorAPI) CreateMonitor(ctx context.Context, body io.Reader) (*Response, error) {
	req, err := http.NewRequest(http.MethodPost, monitorBase, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) GetMonitor(ctx context.Context, monitorId string) (*Response, error) {
	req, err := http.NewRequest(http.MethodGet, getMonitorPath(monitorId), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) UpdateMonitor(ctx context.Context, monitorId string, body io.Reader) (*Response, error) {
	req, err := http.NewRequest(http.MethodPut, getMonitorPath(monitorId), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func (m *MonitorAPI) DeleteMonitor(ctx context.Context, monitorId string) (*Response, error) {
	req, err := http.NewRequest(http.MethodDelete, getMonitorPath(monitorId), nil)
	if err != nil {
		return nil, err
	}
	res, err := m.Perform(req)
	return (*Response)(res), err
}

func getNotificationPath(channelId string) string {
	return path.Join(notificationBase, channelId)
}

func (d *NotificationAPI) CreateNotification(ctx context.Context, body io.Reader) (*Response, error) {
	req, err := http.NewRequest(http.MethodPost, notificationBase, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *NotificationAPI) GetNotification(ctx context.Context, channelId string) (*Response, error) {
	path := getNotificationPath(channelId)
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *NotificationAPI) ListNotifications(ctx context.Context) (*Response, error) {
	req, err := http.NewRequest(http.MethodGet, notificationBase, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *NotificationAPI) UpdateNotification(ctx context.Context, channelId string, body io.Reader) (*Response, error) {
	path := getNotificationPath(channelId)
	req, err := http.NewRequest(http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (d *NotificationAPI) DeleteNotification(ctx context.Context, channelId string) (*Response, error) {
	path := getNotificationPath(channelId)
	req, err := http.NewRequest(http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	res, err := d.Perform(req)
	return (*Response)(res), err
}

func (a *AlertAPI) ListAlerts(ctx context.Context) (*Response, error) {
	path := path.Join(monitorBase, "alerts")
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *AlertAPI) AcknowledgeAlert(ctx context.Context, monitorId string, body io.Reader) (*Response, error) {
	path := path.Join(getMonitorPath(monitorId), "_acknowledge", "alerts")
	req, err := http.NewRequest(http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}
