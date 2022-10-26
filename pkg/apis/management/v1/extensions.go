package v1

func (gc *GatewayConfig) YAMLDocuments() [][]byte {
	docs := [][]byte{}
	for _, doc := range gc.Documents {
		docs = append(docs, doc.Yaml)
	}
	return docs
}

func (cl *CapabilityList) Names() []string {
	names := []string{}
	for _, c := range cl.Items {
		names = append(names, c.Details.Name)
	}
	return names
}
