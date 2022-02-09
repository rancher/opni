package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	registerapi "github.com/rancher/opni/pkg/multicluster/api"
)

const (
	headerContentType = "Content-Type"

	jsonContentHeader = "application/json"
)

var (
	httpClient = &http.Client{
		Timeout: time.Second * 10,
	}
)

func RegisterCluster(name string, apiAddress string) (string, error) {
	body := registerapi.ClusterOp{
		Name: name,
	}
	url, err := url.ParseRequestURI(apiAddress)
	if err != nil {
		return "", err
	}

	clusterPath, _ := url.Parse("/cluster")

	url = url.ResolveReference(clusterPath)
	req, err := http.NewRequest(http.MethodPut, url.String(), opensearchutil.NewJSONReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Add(headerContentType, jsonContentHeader)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("response was %d: %s", resp.StatusCode, body)
	}

	clusterResponse := &registerapi.CreateResponse{}

	err = json.NewDecoder(resp.Body).Decode(clusterResponse)
	if err != nil {
		return "", err
	}

	return clusterResponse.ID, nil
}

func FetchIndexingCredentials(clusterID string, apiAddress string) (username string, password string, retErr error) {
	url, retErr := url.ParseRequestURI(apiAddress)
	if retErr != nil {
		return
	}

	path, _ := url.Parse(fmt.Sprintf("/credentials?id=%s", clusterID))

	url = url.ResolveReference(path)

	// Wait for cluster to be ready in the upstream
	var resp *http.Response
	for {
		Log.Info("waiting for cluster to register")
		resp, retErr = httpClient.Get(url.String())
		if retErr != nil {
			return
		}
		if resp.StatusCode != http.StatusAccepted {
			break
		}
		resp.Body.Close()
		time.Sleep(5 * time.Second)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return username, password, err
		}
		return username, password, fmt.Errorf("response was %d: %s", resp.StatusCode, body)
	}

	jsonResponse := &registerapi.Credentials{}

	retErr = json.NewDecoder(resp.Body).Decode(jsonResponse)
	if retErr != nil {
		return
	}

	username = jsonResponse.Username
	password = jsonResponse.Password

	return
}
