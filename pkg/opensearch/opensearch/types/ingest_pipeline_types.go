package types

type IngestPipeline struct {
	Description string      `json:"description,omitempty"`
	Processors  []Processor `json:"processors,omitempty"`
}

type Processor struct {
	OpniLoggingProcessor *OpniProcessorConfig `json:"opni-logging-processor,omitempty"`
	OpniPreProcessor     *OpniProcessorConfig `json:"opnipre,omitempty"`
}

type OpniProcessorConfig struct {
}
