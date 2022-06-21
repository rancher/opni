package validation_test

import (
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/validation"
)

var (
	// names that match labelNameRegex
	goodLabelNames = []string{
		"foo",
		"foo.com/_bar",
		"foo.com/bar...",
	}

	// names that do not match labelNameRegex
	badLabelNames = []string{
		"_foo.com/bar",
		"foo.com!",
		"",
		"😎",
		string(make([]byte, 65)),
	}

	// names that match labelValueRegex
	goodLabelValues = []string{
		"foo",
		"foo.com",
		"foo_bar",
	}

	// names that do not match labelValueRegex
	badLabelValues = []string{
		"foo!",
		"foo.com/bar",
		"_foo",
		"",
		"😎",
		string(make([]byte, 65)),
	}

	// names that match idRegex
	goodIDs = []string{
		"foo",
		"_foo",
		"foo.com_bar...",
		uuid.NewString(),
	}

	// names that do not match idRegex
	badIDs = []string{
		"foo!",
		"foo.com/bar",
		"",
		"😎",
		"{foo}",
		".",
		"..",
		string(make([]byte, 151)),
	}

	// names that match subjectNameRegex
	goodSubjectNames = []string{
		"foo",
		"{foo}",
		"foo_bar-baz.com/quux",
		"_foo",
		"😎",
	}

	// names that do not match subjectNameRegex
	badSubjectNames = []string{
		`"foo"`,
		`foo.com\bar`,
		`foo'`,
		"foo bar",
		"foo*",
		"",
		string(make([]byte, 257)),
	}
)

func labelNameRegexEntries() []TableEntry {
	entries := []TableEntry{}
	for _, name := range goodLabelNames {
		entries = append(entries, Entry(nil, name, nil))
	}
	for _, name := range badLabelNames {
		entries = append(entries, Entry(nil, name, validation.ErrInvalidLabelName))
	}
	return entries
}

func labelValueRegexEntries() []TableEntry {
	entries := []TableEntry{}
	for _, value := range goodLabelValues {
		entries = append(entries, Entry(nil, value, nil))
	}
	for _, value := range badLabelValues {
		entries = append(entries, Entry(nil, value, validation.ErrInvalidLabelValue))
	}
	return entries
}

func idRegexEntries() []TableEntry {
	entries := []TableEntry{}
	for _, id := range goodIDs {
		entries = append(entries, Entry(nil, id, nil))
	}
	for _, id := range badIDs {
		entries = append(entries, Entry(nil, id, validation.ErrInvalidID))
	}
	return entries
}

func subjectNameRegexEntries() []TableEntry {
	entries := []TableEntry{}
	for _, name := range goodSubjectNames {
		entries = append(entries, Entry(nil, name, nil))
	}
	for _, name := range badSubjectNames {
		entries = append(entries, Entry(nil, name, validation.ErrInvalidSubjectName))
	}
	return entries
}

type testValidator struct{}

func (t *testValidator) Validate() error {
	return errors.New("test")
}

var _ = Describe("Validation", Label("unit"), func() {
	Specify("the Validate helper function should work", func() {
		validator := &testValidator{}
		Expect(validation.Validate(validator)).To(MatchError("test"))
	})
	DescribeTable("Label validation",
		func(labels map[string]string, err error) {
			e := validation.ValidateLabels(labels)
			if err != nil {
				Expect(e).To(MatchError(err))
			} else {
				Expect(e).To(BeNil())
			}
		},
		Entry(nil, map[string]string{}, nil),
		Entry(nil, map[string]string{"foo": "bar"}, nil),
		Entry(nil, map[string]string{"foo.com/_bar": "foo.baz_quux"}, nil),
		Entry(nil, map[string]string{"foo.com/bar...": "foo.com/baz"}, validation.ErrInvalidLabelValue),
		Entry(nil, map[string]string{"foo.com!": "baz"}, validation.ErrInvalidLabelName),
		Entry(nil, map[string]string{"": "baz"}, validation.ErrInvalidLabelName),
		Entry(nil, map[string]string{"foo": ""}, validation.ErrInvalidLabelValue),
	)
	DescribeTable("Label name validation",
		func(name string, err error) {
			e := validation.ValidateLabelName(name)
			if err != nil {
				Expect(e).To(MatchError(err))
			} else {
				Expect(e).To(BeNil())
			}
		},
		labelNameRegexEntries(),
	)
	DescribeTable("Label value validation",
		func(value string, err error) {
			e := validation.ValidateLabelValue(value)
			if err != nil {
				Expect(e).To(MatchError(err))
			} else {
				Expect(e).To(BeNil())
			}
		},
		labelValueRegexEntries(),
	)
	DescribeTable("ID validation",
		func(name string, err error) {
			e := validation.ValidateID(name)
			if err != nil {
				Expect(e).To(MatchError(err))
			} else {
				Expect(e).To(BeNil())
			}
		},
		idRegexEntries(),
	)
	DescribeTable("Subject validation",
		func(subject string, err error) {
			e := validation.ValidateSubject(subject)
			if err != nil {
				Expect(e).To(MatchError(err))
			} else {
				Expect(e).To(BeNil())
			}
		},
		subjectNameRegexEntries(),
	)
})
