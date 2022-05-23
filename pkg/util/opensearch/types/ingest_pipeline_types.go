package types

type IngestPipeline struct {
	Description string      `json:"description,omitempty"`
	Processors  []Processor `json:"processors,omitempty"`
}

type Processor struct {
	OpniPreProcessor *OpniPreProcessor `json:"opnipre,omitempty"`
}

type OpniPreProcessor struct {
	Field       string `json:"field,omitempty"`
	TargetField string `json:"target_field,omitempty"`
}
