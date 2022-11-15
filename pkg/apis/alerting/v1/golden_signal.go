package v1

const SaturationGoldenSignal = "saturation"
const ErrorGoldenSignal = "error"
const LatencyGoldenSignal = "latency"
const TrafficGoldenSignal = "traffic"
const CustomGoldenSignal = "custom"

type IndexableMetric interface {
	GoldenSignal() string
	AlertType() string
}

var _ IndexableMetric = &AlertConditionCPUSaturation{}
var _ IndexableMetric = &AlertConditionMemorySaturation{}
var _ IndexableMetric = &AlertConditionFilesystemSaturation{}
var _ IndexableMetric = &AlertConditionPrometheusQuery{}
var _ IndexableMetric = &AlertConditionKubeState{}

func (a *AlertConditionCPUSaturation) GoldenSignal() string {
	return SaturationGoldenSignal
}

func (a *AlertConditionCPUSaturation) AlertType() string {
	return "cpu_saturation"
}

func (a *AlertConditionMemorySaturation) GoldenSignal() string {
	return SaturationGoldenSignal
}

func (a *AlertConditionMemorySaturation) AlertType() string {
	return "memory_saturation"
}

func (a *AlertConditionFilesystemSaturation) GoldenSignal() string {
	return SaturationGoldenSignal
}

func (a *AlertConditionFilesystemSaturation) AlertType() string {
	return "fs_saturation"
}

func (a *AlertConditionPrometheusQuery) GoldenSignal() string {
	return CustomGoldenSignal
}

func (a *AlertConditionPrometheusQuery) AlertType() string {
	return "prometheus_query"
}

func (a *AlertConditionKubeState) GoldenSignal() string {
	return ErrorGoldenSignal
}

func (a *AlertConditionKubeState) AlertType() string {
	return "kube_state"
}
