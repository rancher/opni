package validation

import (
	"fmt"
	"regexp"
)

var (
	nameConstraint               = `can only contain alphanumeric characters, '-', '_', and '.'`
	annoyingCharactersConstraint = `cannot contain '\', '*', '"', '''`
	lengthConstraint             = func(i int) string { return fmt.Sprintf("must be between 1-%d characters in length", i) }

	ErrMissingRequiredField = Error("missing required field")
	ErrInvalidValue         = Error("invalid value")
	ErrInvalidLabelName     = Errorf("label names %s, and %s", nameConstraint, lengthConstraint(63))
	ErrInvalidLabelValue    = Errorf("label values %s, and %s", nameConstraint, lengthConstraint(63))
	ErrInvalidName          = Errorf("names %s, and %s", nameConstraint, lengthConstraint(63))
	ErrInvalidRoleName      = Errorf("role names %s, and %s", nameConstraint, lengthConstraint(63))
	ErrInvalidSubjectName   = Errorf("subject names %s and %s", lengthConstraint(127), annoyingCharactersConstraint)
	ErrInvalidID            = Errorf("ids %s, and %s", nameConstraint, lengthConstraint(127))

	labelNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9-_./]{1,63}$`)
	labelValueRegex  = regexp.MustCompile(`^[a-zA-Z0-9-_.]{1,63}$`)
	nameRegex        = regexp.MustCompile(`^[a-zA-Z0-9-_.]{1,63}$`)
	idRegex          = regexp.MustCompile(`^[a-zA-Z0-9-_.]{1,127}$`)
	subjectNameRegex = regexp.MustCompile(`^[^\\*"']{1,254}$`)
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

func ValidateName(name string) error {
	if !nameRegex.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

func ValidateRoleName(name string) error {
	if !nameRegex.MatchString(name) {
		return ErrInvalidRoleName
	}
	return nil
}

func ValidateID(id string) error {
	if !idRegex.MatchString(id) {
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
