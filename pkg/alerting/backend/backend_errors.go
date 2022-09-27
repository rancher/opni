package backend

import "regexp"

const NoSmartHostSet = "no global SMTP smarthost set"

const NoSMTPFromSet = "no global SMTP from set"

var FieldNotFound = regexp.MustCompile("line [0-9]+: field .* not found in type config.plain")
