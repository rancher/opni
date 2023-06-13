package types

type IngestPipeline struct {
	Description string      `json:"description,omitempty"`
	Processors  []Processor `json:"processors,omitempty"`
}

type Processor struct {
	OpniLoggingProcessor *OpniProcessorConfig `json:"opni-logging-processor,omitempty"`
	OpniPreProcessor     *OpniProcessorConfig `json:"opnipre,omitempty"`
	TextEmbedding        *TextEmbeddingConfig `json:"text_embedding,omitempty"`
}

type OpniProcessorConfig struct {
}

type TextEmbeddingConfig struct {
	ModelId       string    `json:"model_id,omitempty"`
	FieldMap      *FieldMap `json:"field_map,omitempty"`
	IgnoreFailure bool      `json:"ignore_failure,omitempty"`
}

type FieldMap struct {
	Log string `json:"log,omitempty"`
}
