package compat_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/metrics/compat"
)

var _ = Describe("Prometheus Query response unmarshalling", Label("unit"), func() {
	It("should unmarshal scalar responses", func() {
		scalarData := model.Scalar{
			Value:     1,
			Timestamp: model.Time(1),
		}

		scalarDataWrapper := struct {
			Type   model.ValueType `json:"resultType"`
			Result json.RawMessage `json:"result"`
		}{}
		scalarDataWrapper.Type = model.ValScalar
		scalarDataBytes, err := json.Marshal(scalarData)
		Expect(err).To(Succeed())
		scalarDataWrapper.Result = scalarDataBytes

		scalarDataWrapperBytes, err := json.Marshal(scalarDataWrapper)
		Expect(err).To(Succeed())

		promResp := compat.ApiResponse{
			Status:    "success",
			Data:      scalarDataWrapperBytes,
			ErrorType: "",
			Error:     "",
			Warnings:  []string{},
		}

		promRespData, err := json.Marshal(promResp)
		Expect(err).To(Succeed())

		qr, err := compat.UnmarshalPrometheusResponse(promRespData)
		Expect(err).To(Succeed())
		Expect(qr.Type).To(Equal(model.ValScalar))
		Expect(qr.V).To(Equal(&scalarData))

		vector, err := qr.GetVector()
		Expect(err).NotTo(Succeed())
		Expect(vector).To(BeNil())

		matrix, err := qr.GetMatrix()
		Expect(err).NotTo(Succeed())
		Expect(matrix).To(BeNil())

		scalar, err := qr.GetScalar()
		Expect(err).To(Succeed())
		Expect(scalar.Value).To(Equal(scalarData.Value))
		Expect(scalar.Timestamp).To(Equal(scalarData.Timestamp))

		samples := qr.LinearSamples()
		Expect(samples).To(HaveLen(1))
		Expect(samples[0].Value).To(Equal(float64(scalarData.Value)))
		Expect(samples[0].Timestamp).To(Equal(int64(scalarData.Timestamp)))
	})

	It("should unmarshal vector responses", func() {
		vectorData := model.Vector{
			{
				Value:     1,
				Timestamp: model.Time(1),
			},
			{
				Value:     2,
				Timestamp: model.Time(0),
			},
		}

		dataWrapper := struct {
			Type   model.ValueType `json:"resultType"`
			Result json.RawMessage `json:"result"`
		}{}
		dataWrapper.Type = model.ValVector
		scalarDataBytes, err := json.Marshal(vectorData)
		Expect(err).To(Succeed())
		dataWrapper.Result = scalarDataBytes

		scalarDataWrapperBytes, err := json.Marshal(dataWrapper)
		Expect(err).To(Succeed())

		promResp := compat.ApiResponse{
			Status:    "success",
			Data:      scalarDataWrapperBytes,
			ErrorType: "",
			Error:     "",
			Warnings:  []string{},
		}

		promRespData, err := json.Marshal(promResp)
		Expect(err).To(Succeed())

		qr, err := compat.UnmarshalPrometheusResponse(promRespData)
		Expect(err).To(Succeed())
		Expect(qr.Type).To(Equal(model.ValVector))

		scalar, err := qr.GetScalar()
		Expect(err).NotTo(Succeed())
		Expect(scalar).To(BeNil())

		matrix, err := qr.GetMatrix()
		Expect(err).NotTo(Succeed())
		Expect(matrix).To(BeNil())

		vector, err := qr.GetVector()
		Expect(err).To(Succeed())
		Expect(vector).NotTo(BeNil())

		samples := qr.LinearSamples()
		Expect(samples).To(HaveLen(2))
		Expect(samples[0].Value).To(Equal(float64(vectorData[1].Value)))
		Expect(samples[0].Timestamp).To(Equal(int64(vectorData[1].Timestamp)))
		Expect(samples[1].Value).To(Equal(float64(vectorData[0].Value)))
		Expect(samples[1].Timestamp).To(Equal(int64(vectorData[0].Timestamp)))
	})

	It("should unmarshal matrix responses", func() {
		matrixData := model.Matrix{
			{
				Values: []model.SamplePair{
					{
						Value:     1,
						Timestamp: model.Time(2),
					},

					{
						Value:     2,
						Timestamp: model.Time(0),
					},
				},
			},
			{
				Values: []model.SamplePair{
					{
						Value:     3,
						Timestamp: model.Time(1),
					},
				},
			},
		}

		dataWrapper := struct {
			Type   model.ValueType `json:"resultType"`
			Result json.RawMessage `json:"result"`
		}{}
		dataWrapper.Type = model.ValMatrix
		scalarDataBytes, err := json.Marshal(matrixData)
		Expect(err).To(Succeed())
		dataWrapper.Result = scalarDataBytes

		scalarDataWrapperBytes, err := json.Marshal(dataWrapper)
		Expect(err).To(Succeed())

		promResp := compat.ApiResponse{
			Status:    "success",
			Data:      scalarDataWrapperBytes,
			ErrorType: "",
			Error:     "",
			Warnings:  []string{},
		}

		promRespData, err := json.Marshal(promResp)
		Expect(err).To(Succeed())

		qr, err := compat.UnmarshalPrometheusResponse(promRespData)
		Expect(err).To(Succeed())
		Expect(qr.Type).To(Equal(model.ValMatrix))

		scalar, err := qr.GetScalar()
		Expect(err).NotTo(Succeed())
		Expect(scalar).To(BeNil())

		vector, err := qr.GetVector()
		Expect(err).NotTo(Succeed())
		Expect(vector).To(BeNil())

		matrix, err := qr.GetMatrix()
		Expect(err).To(Succeed())
		Expect(matrix).NotTo(BeNil())

		samples := qr.LinearSamples()
		Expect(samples).To(HaveLen(3))
		Expect(samples[0].Value).To(Equal(float64(2)))
		Expect(samples[0].Timestamp).To(Equal(int64(0)))
		Expect(samples[1].Value).To(Equal(float64(3)))
		Expect(samples[1].Timestamp).To(Equal(int64(1)))
		Expect(samples[2].Value).To(Equal(float64(1)))
		Expect(samples[2].Timestamp).To(Equal(int64(2)))
	})
})
