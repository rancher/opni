package gateway

import (
    "context"
    "errors"
    "time"
    "encoding/json"
    "net/http"

    "github.com/nats-io/nats.go"
    "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metricai "github.com/rancher/opni/plugins/aiops/pkg/apis/metricai"
	// "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
    
)
var serverUrl = "http://metric-ai-service:10010"

func (p *AIOpsPlugin) ListJobs(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIJobList, error) {
    // Implement the logic to list jobs here
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobs for metricAI: %v", err)
	}
    jobs, err := metricAIKeyValue.Keys()
    if err != nil {
        if errors.Is(err, nats.ErrKeyNotFound) {
			return &metricai.MetricAIJobList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to ListJobs for metricAI: %v", err)
    }
    var jobArray []*metricai.MetricAIJobId
    for _, j := range jobs {
        jobArray = append(jobArray, &metricai.MetricAIJobId{
            JobId: j,
        })
    }
    return &metricai.MetricAIJobList{Items: jobArray,}, nil
}

func (p *AIOpsPlugin) SubmitJob(ctx context.Context, jobRequest *metricai.MetricAIJobRequest) (*metricai.MetricAIJobSubmitStatus, error) {
    var httpClient = &http.Client{Timeout: 10 * time.Second}
    resp, err := httpClient.Get(serverUrl+ "/create_task/" + jobRequest.ClusterId + "/" + jobRequest.Namespace)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to SubmitJob for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var res map[string]interface{}
	
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %v", err)
	}
    return &metricai.MetricAIJobSubmitStatus{JobId: res["retrive_id"].(string), JobSubmittedTime: res["submitted time"].(string), Status: "Success"}, nil
}

func (p *AIOpsPlugin) DeleteJobs(ctx context.Context, jobId *metricai.MetricAIJobId) (*metricai.MetricAIJobDeleteStatus, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobs for metricAI: %v", err)
	}
    if _, err := metricAIKeyValue.Get(jobId.JobId); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return &metricai.MetricAIJobDeleteStatus{Status: "Failed", Description: "The key :" + jobId.JobId +" doesn't exist"}, nil
		}
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobs for metricAI: %v", err)
	}
    metricAIKeyValue.Delete(jobId.JobId)
    return &metricai.MetricAIJobDeleteStatus{Status: "Success", Description: "The key :" + jobId.JobId +" is deleted"}, nil
}

func (p *AIOpsPlugin) GetJobResult(ctx context.Context, jobId *metricai.MetricAIJobId) (*metricai.MetricAIGetJobResult, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobs for metricAI: %v", err)
	}
    jobRes, err := metricAIKeyValue.Get(jobId.JobId)
    if err != nil {
        if errors.Is(err, nats.ErrKeyNotFound) {
			return &metricai.MetricAIGetJobResult{Status: "Failed", Description: "The key :" + jobId.JobId +" doesn't exist"}, nil
		}
		return nil, status.Errorf(codes.Internal, "Failed to GetJobResult for metricAI: %v", err)
    }
    var res map[string]interface{}
    if err := json.Unmarshal(jobRes.Value(),&res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal JobResults from Jetstream for metricAI: %v", err)
	}
	resResult, _ := json.Marshal(res["result"].(map[string]interface{}))

    var jobStat = metricai.MetricAIJobStatus{
        JobSubmittedTime: float32(res["submitted_time"].(float64)),
        JobId:     res["task_id"].(string),
        JobStatus:     res["status"].(string),
        JobResult:     string(resResult),
    }  
	return &metricai.MetricAIGetJobResult{
		Status: "Success",
        Description: "",
        JobStatus: &jobStat,
	}, nil


}