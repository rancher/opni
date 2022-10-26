package validation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	nameConstraint               = `can only contain alphanumeric characters, '-', '_', and '.', must start with an alphanumeric character`
	annoyingCharactersConstraint = `cannot contain '\', '*', '"', ''', or ' '`
	lengthConstraint             = func(i int) string { return fmt.Sprintf("must be between 1-%d characters in length", i) }

	ErrMissingRequiredField = Error("missing required field")
	ErrInvalidValue         = Error("invalid value")
	ErrDuplicate            = Error("duplicate value in list")
	ErrReadOnlyField        = Error("field is read-only")
	ErrInvalidLabelName     = Errorf("label names %s, and %s", nameConstraint, lengthConstraint(64))
	ErrInvalidLabelValue    = Errorf("label values %s, and %s", nameConstraint, lengthConstraint(64))
	ErrInvalidName          = Errorf("names %s, and %s", nameConstraint, lengthConstraint(64))
	ErrInvalidRoleName      = Errorf("role names %s, and %s", nameConstraint, lengthConstraint(64))
	ErrInvalidSubjectName   = Errorf("subject names %s and %s", lengthConstraint(256), annoyingCharactersConstraint)
	ErrInvalidID            = Errorf("ids %s, and %s", nameConstraint, lengthConstraint(128))

	labelNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-_./]{0,63}$`)
	labelValueRegex  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-_.]{0,63}$`)
	idRegex          = regexp.MustCompile(`^[a-zA-Z0-9-_.\(\)]{1,128}$`)
	subjectNameRegex = regexp.MustCompile(`^[^\\*"'\s]{1,256}$`)
)

type Validator interface {
	Validate() error
}

func ValidateLabels(labels map[string]string) error {
	for k, v := range labels {
		if err := ValidateLabelName(k); err != nil {
			return err
		}
		if err := ValidateLabelValue(v); err != nil {
			return err
		}
	}
	return nil
}

func ValidateLabelName(name string) error {
	if !labelNameRegex.MatchString(name) {
		return ErrInvalidLabelName
	}
	return nil
}

func ValidateLabelValue(value string) error {
	if !labelValueRegex.MatchString(value) {
		return ErrInvalidLabelValue
	}
	return nil
}

func ValidateID(id string) error {
	if !idRegex.MatchString(id) {
		return ErrInvalidID
	}
	// as a special case, ids containing only '.' characters are invalid
	if strings.Repeat(".", len(id)) == id {
		return ErrInvalidID
	}
	return nil
}

func ValidateSubject(subject string) error {
	if !subjectNameRegex.MatchString(subject) {
		return ErrInvalidSubjectName
	}
	return nil
}

func Validate(v Validator) error {
	return v.Validate()
}
