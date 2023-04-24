package gateway

import (
    "context"
    "errors"
    "time"
    "encoding/json"
    "net/http"
    "strings"
    "os"

    "github.com/nats-io/nats.go"
    "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metricai "github.com/rancher/opni/plugins/aiops/pkg/apis/metricai"
	"google.golang.org/protobuf/types/known/emptypb"
    grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
    "github.com/rancher/opni/pkg/resources"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    
)

const serverUrl = "http://metric-ai-service:8090" // URL of the python metric-ai service
const jobRunDelimiter = "=" // delimiter that splits jobid and suffix, in order to save in natsKV
const dashboardNamePrefix = "metricai-"
var dashboardSelector = &metav1.LabelSelector{
    MatchLabels: map[string]string{
        resources.AppNameLabel:  "grafana",
        resources.PartOfLabel:   "opni",
        resources.InstanceLabel: "opni", // TODO: this should be the name of MonitoringCluster
    },
}

type RequestError struct {
	StatusCode int
	Err error
}
func (r *RequestError) Error() string {
	return r.Err.Error()
}


func (p *AIOpsPlugin) CreateGrafanaDashboard(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
    res, err := p.GetJobRunResult(ctx, jobRunId)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to Get JobRunRes for metricAI: %v", err)
	}
    dashboardJson := string(res.JobRunResultDetails)
    dashboardName := strings.ToLower(strings.ReplaceAll(res.JobRunId, jobRunDelimiter, ""))
    grafanaDashboards := []*grafanav1alpha1.GrafanaDashboard{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dashboardNamePrefix+ dashboardName,
				Namespace: os.Getenv("POD_NAMESPACE"),
                Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1alpha1.GrafanaDashboardSpec{
				Json: dashboardJson,
			},
		},
    }
    for _, dashboard := range grafanaDashboards {
        err := p.k8sClient.Create(ctx, dashboard)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "Error Creating Dashboard for metricAI: %v", err)
        }
	}
    return &metricai.MetricAIAPIResponse{ SubmittedTime: time.Now().String(), Description:dashboardJson,Status: "Success"}, nil
}

func (p *AIOpsPlugin) DeleteGrafanaDashboard(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
    res, err := p.GetJobRunResult(ctx, jobRunId)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to Get JobRunRes for metricAI: %v", err)
	}
    dashboardJson := res.JobRunResultDetails
    dashboardName := strings.ToLower(strings.ReplaceAll(res.JobRunId, jobRunDelimiter, ""))
    grafanaDashboards := []*grafanav1alpha1.GrafanaDashboard{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:     dashboardNamePrefix+ dashboardName,
				Namespace: os.Getenv("POD_NAMESPACE"),
                Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1alpha1.GrafanaDashboardSpec{
				Json: dashboardJson,
			},
		},
    }
    for _, dashboard := range grafanaDashboards {
        err := p.k8sClient.Delete(ctx, dashboard)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "Error Deleting Dashboard for metricAI: %v", err)
        }
	}
    return &metricai.MetricAIAPIResponse{ SubmittedTime: time.Now().String(), Description:dashboardJson,Status: "Success"}, nil
}

func (p *AIOpsPlugin) ListClusters(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
    timeout := 10*time.Second
    ctxca, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    httpClient := &http.Client{Timeout: timeout}
    // make the http request with context
	req, err := http.NewRequestWithContext(ctxca, "GET", serverUrl+ "/get_users", nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to form httprequest to ListClusters for metricAI: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to make httprequest to ListClusters for metricAI: %v", err)
	}
	defer resp.Body.Close()

	var result []string
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of ListClusters for metricAI: %v", err)
	}
    return &metricai.MetricAIIdList{Items: result,}, nil
}

func (p *AIOpsPlugin) ListNamespaces(ctx context.Context, clusterId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
    timeout := 10*time.Second
    ctxca, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    httpClient := &http.Client{Timeout: timeout}
    // make the http request with context
	req, err := http.NewRequestWithContext(ctxca, "GET", serverUrl+ "/list_namespace/" + clusterId.Id, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to form httprequest to ListNamespaces for metricAI: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to make httprequest to ListNamespaces for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var result []string
	
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of ListNamespaces for metricAI: %v", err)
	}
    return &metricai.MetricAIIdList{Items: result,}, nil
}

// list keys in the natsKV Job bucket
func (p *AIOpsPlugin) ListJobs(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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

// list keys in the natsKV JobRun bucket
func (p *AIOpsPlugin) ListJobRuns(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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
    timeout := 10*time.Second
    ctxca, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    httpClient := &http.Client{Timeout: timeout}
    // make the http request with context
	req, err := http.NewRequestWithContext(ctxca, "GET", serverUrl+ "/run_job/" + jobRequest.Id, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to form httprequest to RunJob for metricAI: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to make httprequest to RunJob for metricAI: %v", err)
	}
	defer resp.Body.Close()
	var res map[string]interface{}
	
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %v", err)
	}
    return &metricai.MetricAIRunJobResponse{JobRunId: res["JobRunId"].(string), SubmittedTime: time.Now().String(), Status: res["Status"].(string)}, nil
}


func (p *AIOpsPlugin) CreateJob(ctx context.Context, jobRequest *metricai.MetricAICreateJobRequest) (*metricai.MetricAIAPIResponse, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
    metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobRuns for metricAI: %v", err)
	}
    jid := strings.ToLower(jobRequest.JobId)

    if jid == "" { // id can't be empty
        return nil, &RequestError{
            StatusCode: 503,
            Err:        errors.New("jobId can't be empty"),
        }
    }

    if strings.Contains(jid, jobRunDelimiter) { // TODO: should only allow chars include alphanum and - and _
        return nil, &RequestError{
            StatusCode: 503,
            Err:        errors.New("jobId can't contain special char "+jobRunDelimiter),
        }
    }
    
    if _, err := metricAIKeyValue.Get(jid); err == nil { // check if this id exists
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

// delete job_id from the natsKV bucket. This won't delete the job_run attached to this job_id
func (p *AIOpsPlugin) DeleteJob(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJob for metricAI: %v", err)
	}
    metricAIKeyValue.Delete(jid)
    return &metricai.MetricAIAPIResponse{Status: "Success", Description: "The JobId key :" + jid +" is deleted"}, nil
}

// delete a job_run of a job. Won't delete the job itself.
func (p *AIOpsPlugin) DeleteJobRun(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
    metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
    if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobRun for metricAI: %v", err)
	}
    jid := jobRunId.Id
    if _, err := metricAIKeyValue.Get(jid); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, &RequestError{
                StatusCode: 503,
                Err:        errors.New("The jobRunId to delete doesn't exist"),
            }
        }
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobRun for metricAI: %v", err)
	}
    metricAIKeyValue.Delete(jid)
    return &metricai.MetricAIAPIResponse{Status: "Success", Description: "The JobRunId key :" + jid +" is deleted"}, nil
}


func (p *AIOpsPlugin) GetJobRunResult(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIJobRunResult, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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
	return &res, nil

}

func (p *AIOpsPlugin) GetJob(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIJobStatus, error) {
    ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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