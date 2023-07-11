package driverutil_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/plugins/driverutil"
)

var _ = Describe("Option", func() {
	type sampleStruct struct {
		A int    `option:"a"`
		B string `option:"b"`

		cantSet string `option:"cantSet"`
	}

	Describe("Apply", func() {
		When("applying an option", func() {
			It("should set the corresponding field in a struct", func() {
				s := &sampleStruct{}
				opt := driverutil.NewOption[int]("a", 123)

				err := opt.Apply(s)

				Expect(err).NotTo(HaveOccurred())
				Expect(s.A).To(Equal(123))
			})
			When("the option doesn't match the field type", func() {
				It("should return an error", func() {
					s := &sampleStruct{}
					opt := driverutil.NewOption[string]("a", "123")

					err := opt.Apply(s)

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("mismatched option types for key \"a\": int != string"))
				})
			})
			When("the key doesn't match any fields", func() {
				It("should skip the option", func() {
					s := &sampleStruct{A: 123}
					opt := driverutil.NewOption[int]("c", 456)

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.A).To(Equal(123))
				})
			})
			When("the key is empty", func() {
				It("should skip the option", func() {
					s := &sampleStruct{A: 123}
					opt := driverutil.NewOption[int]("", 456)

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.A).To(Equal(123))
				})
			})
			When("the field cannot be set", func() {
				It("should skip the option", func() {
					s := &sampleStruct{}
					opt := driverutil.NewOption[string]("cantSet", "value")

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.cantSet).To(Equal(""))

				})
			})
			When("the destination argument is not a pointer to a struct", func() {
				It("should panic", func() {
					Expect(func() {
						driverutil.NewOption[int]("a", 123).Apply(sampleStruct{})
					}).To(Panic())
					Expect(func() {
						driverutil.NewOption[int]("a", 123).Apply(new(int))
					}).To(Panic())
				})
			})
			When("the option value is the zero value for its type", func() {
				It("should not set the field", func() {
					s := &sampleStruct{A: 123}
					opt := driverutil.NewOption[int]("a", 0)

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.A).To(Equal(123))
				})
			})
		})
	})

	Describe("ApplyOptions", func() {
		It("should be able to apply multiple options at once", func() {
			s := &sampleStruct{}
			opts := []driverutil.Option{
				driverutil.NewOption[int]("a", 123),
				driverutil.NewOption[string]("b", "hello"),
			}

			err := driverutil.ApplyOptions(s, opts...)

			Expect(err).NotTo(HaveOccurred())
			Expect(s.A).To(Equal(123))
			Expect(s.B).To(Equal("hello"))
		})

		It("should return all encountered errors when applying options", func() {
			s := &sampleStruct{}
			opts := []driverutil.Option{
				driverutil.NewOption[int]("b", 123),    // mismatched type
				driverutil.NewOption[string]("a", "x"), // mismatched type
			}

			err := driverutil.ApplyOptions(s, opts...)

			Expect(err).To(HaveOccurred())
			errs := err.(interface{ Unwrap() []error }).Unwrap()
			Expect(errs).To(ConsistOf(
				errors.New(`mismatched option types for key "b": string != int`),
				errors.New(`mismatched option types for key "a": int != string`),
			))
		})
	})
})
