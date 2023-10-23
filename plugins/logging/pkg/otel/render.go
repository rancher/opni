package otel

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

const (
	pbContentType   = "application/x-protobuf"
	jsonContentType = "application/json"
)

type protoJSON struct {
	Data protoreflect.ProtoMessage
}

func (p protoJSON) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = []string{jsonContentType}
	}
}

func (p protoJSON) Render(w http.ResponseWriter) error {
	p.WriteContentType(w)

	bytes, err := protojson.Marshal(p.Data)
	if err != nil {
		return err
	}

	_, err = w.Write(bytes)
	return err
}

func (f *OTELForwarder) renderProto(c *gin.Context) {
	body, err := readBody(c)
	if err != nil {
		f.lg.Error(fmt.Sprintf("failed to read body: %v", err))
		c.Status(http.StatusBadRequest)
		return
	}

	req := &collogspb.ExportLogsServiceRequest{}
	err = proto.Unmarshal(body, req)
	if err != nil {
		f.lg.Error(fmt.Sprintf("failed to unmarshal body: %v", err))
		f.lg.Debug(fmt.Sprintf("body: %x", body))
		c.Status(http.StatusBadRequest)
		return
	}

	otlpResp, err := f.forwardLogs(c.Request.Context(), req)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Render(http.StatusOK, render.ProtoBuf{
		Data: otlpResp,
	})
}

func (f *OTELForwarder) renderProtoJSON(c *gin.Context) {
	body, err := readBody(c)
	if err != nil {
		f.lg.Error(fmt.Sprintf("failed to read body: %v", err))
		c.Status(http.StatusBadRequest)
		return
	}

	req := &collogspb.ExportLogsServiceRequest{}
	err = protojson.Unmarshal(body, req)
	if err != nil {
		f.lg.Error(fmt.Sprintf("failed to unmarshal body: %v", err))
		c.Status(http.StatusBadRequest)
		return
	}

	otlpResp, err := f.forwardLogs(c.Request.Context(), req)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Render(http.StatusOK, protoJSON{
		Data: otlpResp,
	})
}

func readBody(c *gin.Context) ([]byte, error) {
	bodyReader := c.Request.Body
	if c.GetHeader("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(c.Request.Body)
		if err != nil {
			return []byte{}, err
		}
		defer gr.Close()
		bodyReader = gr
	}
	return io.ReadAll(bodyReader)
}
