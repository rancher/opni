package oci_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/oci"
)

var _ = Describe("OCI", Label("unit"), func() {
	When("image string does not contain registry", func() {
		When("image tag is present", func() {
			It("should parse the image correctly", func() {
				image := "opni/minimal:v1.0.0"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal(""))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal("v1.0.0"))
			})
		})
		When("image digest is present", func() {
			It("should parse the image correctly", func() {
				image := "opni/minimal@sha256:1234567890"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal(""))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal("sha256:1234567890"))
			})
		})
		When("neither tag nor digest is present", func() {
			It("should parse the image correctly", func() {
				image := "opni/minimal"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal(""))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal(""))
			})
		})
	})
	When("image string contains registry", func() {
		When("image tag is present", func() {
			It("should parse the image correctly", func() {
				image := "quay.io/opni/minimal:v1.0.0"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal("quay.io"))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal("v1.0.0"))
			})
		})
		When("image digest is present", func() {
			It("should parse the image correctly", func() {
				image := "quay.io/opni/minimal@sha256:1234567890"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal("quay.io"))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal("sha256:1234567890"))
			})
		})
		When("neither tag nor digest is present", func() {
			It("should parse the image correctly", func() {
				image := "quay.io/opni/minimal"
				parsed := oci.Parse(image)
				Expect(parsed.Registry).To(Equal("quay.io"))
				Expect(parsed.Repository).To(Equal("opni/minimal"))
				Expect(parsed.Digest).To(Equal(""))
			})
		})
	})
	When("image string contains registry and port", func() {
		It("should parse the image correctly", func() {
			image := "quay.io:5000/opni/minimal:v1.0.0"
			parsed := oci.Parse(image)
			Expect(parsed.Registry).To(Equal("quay.io:5000"))
			Expect(parsed.Repository).To(Equal("opni/minimal"))
			Expect(parsed.Digest).To(Equal("v1.0.0"))
		})
	})
	When("image string contains registry and a simple repository", func() {
		It("should parse the image correctly", func() {
			image := "quay.io/opni"
			parsed := oci.Parse(image)
			Expect(parsed.Registry).To(Equal("quay.io"))
			Expect(parsed.Repository).To(Equal("opni"))
			Expect(parsed.Digest).To(Equal(""))
		})
	})
	When("image string contains a simple repository", func() {
		It("should parse the image correctly", func() {
			image := "opni"
			parsed := oci.Parse(image)
			Expect(parsed.Registry).To(Equal(""))
			Expect(parsed.Repository).To(Equal("opni"))
			Expect(parsed.Digest).To(Equal(""))
		})
	})
	When("image string contains a simple repository and a tag", func() {
		It("should parse the image correctly", func() {
			image := "opni:v1.0.0"
			parsed := oci.Parse(image)
			Expect(parsed.Registry).To(Equal(""))
			Expect(parsed.Repository).To(Equal("opni"))
			Expect(parsed.Digest).To(Equal("v1.0.0"))
		})
	})

	When("image has a registry and no tag/digest", func() {
		image := oci.Image{
			Registry:   "quay.io",
			Repository: "opni/minimal",
			Digest:     "",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("quay.io/opni/minimal"))
			Expect(image.String()).To(Equal("quay.io/opni/minimal"))
		})
	})
	When("image has a registry and a tag", func() {
		image := oci.Image{
			Registry:   "quay.io",
			Repository: "opni/minimal",
			Digest:     "v1.0.0",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("quay.io/opni/minimal"))
			Expect(image.String()).To(Equal("quay.io/opni/minimal:v1.0.0"))
		})
	})
	When("image has a registry and a digest", func() {
		image := oci.Image{
			Registry:   "quay.io",
			Repository: "opni/minimal",
			Digest:     "sha256:1234567890",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("quay.io/opni/minimal"))
			Expect(image.String()).To(Equal("quay.io/opni/minimal@sha256:1234567890"))
		})
	})
	When("image has no registry and no tag/digest", func() {
		image := oci.Image{
			Registry:   "",
			Repository: "opni/minimal",
			Digest:     "",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("opni/minimal"))
			Expect(image.String()).To(Equal("opni/minimal"))
		})
	})
	When("image has no registry and a tag", func() {
		image := oci.Image{
			Registry:   "",
			Repository: "opni/minimal",
			Digest:     "v1.0.0",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("opni/minimal"))
			Expect(image.String()).To(Equal("opni/minimal:v1.0.0"))
		})
	})
	When("image has a registry and a digest", func() {
		image := oci.Image{
			Registry:   "",
			Repository: "opni/minimal",
			Digest:     "sha256:1234567890",
		}
		It("should return the correct image strings", func() {
			Expect(image.Path()).To(Equal("opni/minimal"))
			Expect(image.String()).To(Equal("opni/minimal@sha256:1234567890"))
		})
	})

})
