package gateway

import (
    "context"
    "errors"
    "time"
    "encoding/json"
    "net/http"
    "strings"

    "github.com/nats-io/nats.go"
    "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metricai "github.com/rancher/opni/plugins/aiops/pkg/apis/metricai"
	"google.golang.org/protobuf/types/known/emptypb"
    
)

const serverUrl = "http://metric-ai-service:8090"
const jobRunDelimiter = "~"

type RequestError struct {
	StatusCode int
	Err error
}
func (r *RequestError) Error() string {
	return r.Err.Error()
}

func (p *AIOpsPlugin) ListClusters(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
    var httpClient = &http.Client{Timeout: 10 * time.Second}
    resp, err := httpClient.Get(serverUrl+ "/get_users" )
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to RunJob for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var result []string
	
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %v", err)
	}
    return &metricai.MetricAIIdList{Items: result,}, nil
}

func (p *AIOpsPlugin) ListNamespaces(ctx context.Context, clusterId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
    var httpClient = &http.Client{Timeout: 10 * time.Second}
    resp, err := httpClient.Get(serverUrl+ "/list_namespace/" + clusterId.Id )
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to RunJob for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var result []string
	
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %v", err)
	}
    return &metricai.MetricAIIdList{Items: result,}, nil
}


func (p *AIOpsPlugin) ListJobs(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobs for metricAI: %v", err)
	}
    jobs, err := metricAIKeyValue.Keys()
    if err != nil {
        if errors.Is(err, nats.ErrNoKeysFound) {
			return &metricai.MetricAIIdList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to ListJobs for metricAI: %v", err)
    }
    return &metricai.MetricAIIdList{Items: jobs,}, nil
}

func (p *AIOpsPlugin) ListJobRuns(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobRuns for metricAI: %v", err)
	}
    jobruns, err := metricAIKeyValue.Keys()
    if err != nil {
        if errors.Is(err, nats.ErrNoKeysFound) {
			return &metricai.MetricAIIdList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to ListJobRuns for metricAI: %v", err)
    }
    var jobRunIdArray []string
    for _, j := range jobruns {
        if strings.HasPrefix(j, jobId.Id + jobRunDelimiter){
            jobRunIdArray = append(jobRunIdArray, j)
        }
        
    }
    return &metricai.MetricAIIdList{Items: jobRunIdArray,}, nil
}

func (p *AIOpsPlugin) RunJob(ctx context.Context, jobRequest *metricai.MetricAIId) (*metricai.MetricAIRunJobResponse, error) {
    var httpClient = &http.Client{Timeout: 10 * time.Second}
    resp, err := httpClient.Get(serverUrl+ "/run_job/" + jobRequest.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to RunJob for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var res map[string]interface{}
	
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %v", err)
	}
    return &metricai.MetricAIRunJobResponse{JobRunId: res["JobRunId"].(string), SubmittedTime: time.Now().String(), Status: res["Status"].(string)}, nil
}

func (p *AIOpsPlugin) CreateJob(ctx context.Context, jobRequest *metricai.MetricAICreateJobRequest) (*metricai.MetricAIAPIResponse, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobRuns for metricAI: %v", err)
	}
    jid := jobRequest.JobId

    if jid == "" { // id can't be empty
        return nil, &RequestError{
            StatusCode: 503,
            Err:        errors.New("jobId can't be empty"),
        }
    }

    if strings.Contains(jid, jobRunDelimiter) { // no char | for id
        return nil, &RequestError{
            StatusCode: 503,
            Err:        errors.New("jobId can't contain special char "+jobRunDelimiter),
        }
    }
    
    if _, err := metricAIKeyValue.Get(jid); err == nil { // check duplicate id
        return nil, &RequestError{
            StatusCode: 503,
            Err:        errors.New("The jobId to add already exist"),
        }
    }

    job:= make(map[string]interface{})
    job["JobId"] = jid
    job["JobCreateTime"] = time.Now().String()
    job["ClusterId"] = jobRequest.ClusterId
    job["Namespaces"] = jobRequest.Namespaces
    job["JobDescription"] = jobRequest.JobDescription
    jobJsonStr, err := json.Marshal(job)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "Failed to marshal JobStatus for metricAI: %v", err)
    }
    _, err = metricAIKeyValue.Put(jid, []byte(jobJsonStr))
    if err != nil {
        return nil, status.Errorf(codes.Internal, "Failed to CreateJob for metricAI: %v", err)
    }
    return &metricai.MetricAIAPIResponse{ SubmittedTime: time.Now().String(), Status: "Success"}, nil
}

func (p *AIOpsPlugin) DeleteJobs(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJob for metricAI: %v", err)
	}
    jid := jobId.Id
    if _, err := metricAIKeyValue.Get(jid); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, &RequestError{
                StatusCode: 503,
                Err:        errors.New("The jobId to delete doesn't exist"),
            }
        }
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobs for metricAI: %v", err)
	}
    metricAIKeyValue.Delete(jid)
    return &metricai.MetricAIAPIResponse{Status: "Success", Description: "The key :" + jid +" is deleted"}, nil
}

func (p *AIOpsPlugin) GetJobRunResult(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIJobRunResult, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to GetJobRunResult bucket for metricAI: %v", err)
	}
    jid := jobRunId.Id
    jobRes, err := metricAIKeyValue.Get(jid)
    if err != nil {
        if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, &RequestError{
                StatusCode: 503,
                Err:        errors.New("The jobRunId doesn't exist"),
            }
		}
		return nil, status.Errorf(codes.Internal, "Failed to GetJobRunResult key %s for metricAI: %v",jid, err)
    }
    var res = metricai.MetricAIJobRunResult{}
    if err := json.Unmarshal(jobRes.Value(), &res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal GetJobRunResult from Jetstream for metricAI: %v", err)
	}
	// resResult, err := json.Marshal(res["result"].(map[string]interface{}))
    // if err != nil {
    //     return nil, status.Errorf(codes.Internal, "Failed to marshal GetJobRunResult from Jetstream for metricAI: %v", err)
    // }

	return &res, nil

}

func (p *AIOpsPlugin) GetJob(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIJobStatus, error) {
    ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
    metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to GetJob for metricAI: %v", err)
	}
    jid := jobId.Id
    jobRes, err := metricAIKeyValue.Get(jid)
    if err != nil {
        if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, &RequestError{
                StatusCode: 503,
                Err:        errors.New("The jobId doesn't exist"),
            }
		}
		return nil, status.Errorf(codes.Internal, "Failed to GetJob for metricAI: %v", err)
    }
    var res = metricai.MetricAIJobStatus{}
    if err := json.Unmarshal(jobRes.Value(), &res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal JobStatus from Jetstream for metricAI: %v", err)
	}
    return &res, nil

}