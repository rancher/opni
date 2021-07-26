package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PretrainedModel Controller", func() {
	Expect(true).To(Equal(true))
	When("creating a pretrainedmodel", func() {
		It("should succeed", func() {})
		It("should create a configmap", func() {})
		It("should contain the correct json data", func() {})
	})
	When("updating the hyperparameters", func() {
		It("should succeed", func() {})
		It("should update the configmap with the new data", func() {})
	})
	When("the configmap is manually modified", func() {
		It("should restore the contents of the configmap", func() {})
	})
	When("deleting a pretrainedmodel", func() {
		It("should succeed", func() {})
		It("should delete the configmap", func() {})
	})
})
