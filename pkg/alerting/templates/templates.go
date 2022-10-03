package templates

func StrAsTemplateValue(s string) string {
	return "{{ ." + s + " }}"
}

func StrSliceAsTemplates(s []string) []string {
	var templates []string
	for _, v := range s {
		templates = append(templates, StrAsTemplateValue(v))
	}
	return templates
}
