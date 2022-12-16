package patch_test

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zstd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/grpc/encoding"
)

var _ = Describe("Compression", func() {
	It("should enable the zstd compression format in grpc", func() {
		compressor := encoding.GetCompressor("zstd")
		Expect(compressor).NotTo(BeNil())
		Expect(compressor.Name()).To(Equal("zstd"))
	})
	It("should decompress zstd data", func() {
		sampleData := []byte("hello world")

		// compress using the zstd library
		encoded := new(bytes.Buffer)
		encoder, err := zstd.NewWriter(encoded)
		Expect(err).NotTo(HaveOccurred())
		_, err = encoder.Write(sampleData)
		Expect(err).NotTo(HaveOccurred())
		Expect(encoder.Close()).To(Succeed())

		// decompress using the grpc abstraction
		compressor := encoding.GetCompressor("zstd")
		decompressedReader, err := compressor.Decompress(encoded)
		Expect(err).NotTo(HaveOccurred())
		decompressedData, err := io.ReadAll(decompressedReader)
		Expect(err).NotTo(HaveOccurred())

		// compare the results
		Expect(decompressedData).To(Equal(sampleData))
	})
	It("should compress zstd data", func() {
		sampleData := []byte("hello world")

		// compress using the grpc abstraction
		encoded := new(bytes.Buffer)
		compressor := encoding.GetCompressor("zstd")
		compressedWriter, err := compressor.Compress(encoded)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.Copy(compressedWriter, bytes.NewReader(sampleData))
		Expect(err).NotTo(HaveOccurred())
		Expect(compressedWriter.Close()).To(Succeed())

		// decompress using the zstd library
		decoder, err := zstd.NewReader(encoded)
		Expect(err).NotTo(HaveOccurred())
		decoded, err := io.ReadAll(decoder)
		Expect(err).NotTo(HaveOccurred())

		// compare the results
		Expect(decoded).To(Equal(sampleData))
	})
})
