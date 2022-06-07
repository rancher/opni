package v1

func (gc *GatewayConfig) YAMLDocuments() [][]byte {
	docs := [][]byte{}
	for _, doc := range gc.Documents {
		docs = append(docs, doc.Yaml)
	}
	return docs
}
