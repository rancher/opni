package driverutil_test

import (
	"bytes"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/rancher/opni/pkg/plugins/driverutil"
)

var _ = Describe("Option", func() {
	type sampleStruct struct {
		Int       int       `option:"int"`
		Uint      uint32    `option:"uint"`
		String    string    `option:"string"`
		Slice     []string  `option:"slice"`
		Pointer   *bool     `option:"pointer"`
		Interface io.Writer `option:"interface"`

		cantSet string `option:"cantSet"`
	}

	Describe("Apply", func() {
		When("applying an option", func() {
			It("should set the corresponding field in a struct", func() {
				s := &sampleStruct{}

				Expect(driverutil.NewOption("int", 123).Apply(s)).To(Succeed())
				Expect(driverutil.NewOption[uint32]("uint", 123).Apply(s)).To(Succeed())
				Expect(driverutil.NewOption("string", "hello").Apply(s)).To(Succeed())
				Expect(driverutil.NewOption("slice", []string{"a", "b", "c"}).Apply(s)).To(Succeed())
				Expect(driverutil.NewOption("pointer", lo.ToPtr(true)).Apply(s)).To(Succeed())
				Expect(driverutil.NewOption("interface", GinkgoWriter).Apply(s)).To(Succeed())

				Expect(s.Int).To(Equal(123))
				Expect(s.Uint).To(Equal(uint32(123)))
				Expect(s.String).To(Equal("hello"))
				Expect(s.Slice).To(Equal([]string{"a", "b", "c"}))
				Expect(*s.Pointer).To(BeTrue())
				Expect(s.Interface).To(Equal(GinkgoWriter))
			})
			When("the option doesn't match the field type", func() {
				It("should return an error", func() {
					s := &sampleStruct{}
					err := driverutil.NewOption("int", "123").Apply(s)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("mismatched option types for key \"int\": expected int, got string"))

					err = driverutil.NewOption("uint", 123).Apply(s)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("mismatched option types for key \"uint\": expected uint32, got int"))

					err = driverutil.NewOption("interface", bytes.NewReader(nil)).Apply(s)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("mismatched option types for key \"interface\": expected io.Writer, got *bytes.Reader"))
				})
			})
			When("the key doesn't match any fields", func() {
				It("should skip the option", func() {
					s := &sampleStruct{Int: 123}
					opt := driverutil.NewOption[int]("nonexistent", 456)

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.Int).To(Equal(123))
				})
			})
			When("the key is empty", func() {
				It("should skip the option", func() {
					s := &sampleStruct{Int: 123}
					opt := driverutil.NewOption[int]("", 456)

					err := opt.Apply(s)

					Expect(err).NotTo(HaveOccurred())
					Expect(s.Int).To(Equal(123))
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
						driverutil.NewOption[int]("int", 123).Apply(sampleStruct{})
					}).To(Panic())
					Expect(func() {
						driverutil.NewOption[int]("int", 123).Apply(new(int))
					}).To(Panic())
				})
			})
			When("the option value is the zero value for its type", func() {
				It("should not set the field", func() {
					s := &sampleStruct{Int: 123, Pointer: lo.ToPtr(true)}
					opt := driverutil.NewOption[int]("int", 0)
					err := opt.Apply(s)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.Int).To(Equal(123))

					opt2 := driverutil.NewOption[*bool]("pointer", nil)
					err = opt2.Apply(s)
					Expect(err).NotTo(HaveOccurred())
					Expect(*s.Pointer).To(BeTrue())
				})
			})
		})
	})

	Describe("ApplyOptions", func() {
		It("should be able to apply multiple options at once", func() {
			s := &sampleStruct{}
			opts := []driverutil.Option{
				driverutil.NewOption[int]("int", 123),
				driverutil.NewOption[uint32]("uint", 123),
				driverutil.NewOption[string]("string", "hello"),
				driverutil.NewOption[[]string]("slice", []string{"a", "b", "c"}),
				driverutil.NewOption[*bool]("pointer", lo.ToPtr(true)),
			}

			err := driverutil.ApplyOptions(s, opts...)

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Int).To(Equal(123))
			Expect(s.Uint).To(Equal(uint32(123)))
			Expect(s.String).To(Equal("hello"))
			Expect(s.Slice).To(Equal([]string{"a", "b", "c"}))
			Expect(*s.Pointer).To(BeTrue())
		})

		It("should return all encountered errors when applying options", func() {
			s := &sampleStruct{}
			opts := []driverutil.Option{
				driverutil.NewOption[int]("string", 123), // mismatched type
				driverutil.NewOption[string]("int", "x"), // mismatched type
			}

			err := driverutil.ApplyOptions(s, opts...)

			Expect(err).To(HaveOccurred())
			errs := err.(interface{ Unwrap() []error }).Unwrap()
			Expect(errs).To(ConsistOf(
				errors.New(`mismatched option types for key "string": expected string, got int`),
				errors.New(`mismatched option types for key "int": expected int, got string`),
			))
		})
	})
})
