package agent

import (
	"bytes"
	"context"
	"fmt"

	"io"
	"net/http"

	"github.com/golang/snappy"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/prompb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type RemoteReader interface {
	Read(ctx context.Context, endpoint string, request *prompb.ReadRequest) (*prompb.ReadResponse, error)
}

func NewRemoteReader(prometheusClient *http.Client) RemoteReader {
	return &remoteReader{
		prometheusClient: prometheusClient,
	}
}

type remoteReader struct {
	prometheusClient *http.Client
}

func (client *remoteReader) Read(ctx context.Context, endpoint string, readRequest *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	uncompressedData, err := proto.Marshal(protoimpl.X.ProtoMessageV2Of(readRequest))
	if err != nil {
		return nil, fmt.Errorf("unable to marshal remote read readRequest: %w", err)
	}

	compressedData := snappy.Encode(nil, uncompressedData)

	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("unable to create remote read http readRequest: %w", err)
	}

	request.Header.Add("Content-Encoding", "snappy")
	request.Header.Add("Accept-Encoding", "snappy")
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("User-Agent", "Prometheus/xx")
	request.Header.Set("X-Prometheus-Remote-Read-Version", fmt.Sprintf("Prometheus/%s", version.Version))

	request = request.WithContext(ctx)

	response, err := client.prometheusClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not get response from remote read: %w", err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}()

	var reader bytes.Buffer
	_, _ = io.Copy(&reader, response.Body)

	compressedData, err = io.ReadAll(bytes.NewReader(reader.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("error reading http response: %w", err)
	}

	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("endpoint '%s' responded with status code '%d'", endpoint, response.StatusCode)
	}

	uncompressedData, err = snappy.Decode(nil, compressedData)
	if err != nil {
		return nil, fmt.Errorf("unabled to uncompress reponse: %w", err)
	}

	var readResponse prompb.ReadResponse
	err = proto.Unmarshal(uncompressedData, protoimpl.X.ProtoMessageV2Of(&readResponse))
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal remote read reponse: %w", err)
	}

	return &readResponse, nil
}
