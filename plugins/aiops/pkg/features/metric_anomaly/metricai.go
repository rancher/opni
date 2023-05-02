package metric_anomaly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/metricai"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	jobRunDelimiter     = "=" // delimiter that splits jobid and suffix, in order to save in natsKV
	dashboardNamePrefix = "metricai-"
	timeout             = 10 * time.Second
)

func (p *MetricAnomaly) CreateGrafanaDashboard(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
	// dashboardJson is generated in the python service. This function simply create a GrafanaDashboard resource with it and apply it
	res, err := p.GetJobRunResult(ctx, jobRunId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to Get JobRunRes for metricAI: %s", err)
	}
	dashboardJson := res.JobRunResultDetails
	dashboardName := strings.ToLower(strings.ReplaceAll(res.JobRunId, jobRunDelimiter, ""))
	err = p.DashboardDriver.CreateDashboard(dashboardNamePrefix+dashboardName, dashboardJson)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error Creating Dashboard for metricAI: %s", err)
	}

	return &metricai.MetricAIAPIResponse{SubmittedTime: time.Now().String(), Description: dashboardJson, Status: "Success"}, nil
}

func (p *MetricAnomaly) DeleteGrafanaDashboard(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
	// delete the grafanadashboard resource for the given jobrun id
	res, err := p.GetJobRunResult(ctx, jobRunId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to Get JobRunRes for metricAI: %s", err)
	}
	dashboardJson := res.JobRunResultDetails
	dashboardName := strings.ToLower(strings.ReplaceAll(res.JobRunId, jobRunDelimiter, ""))

	p.DashboardDriver.DeleteDashboard(dashboardNamePrefix + dashboardName)

	return &metricai.MetricAIAPIResponse{SubmittedTime: time.Now().String(), Description: dashboardJson, Status: "Success"}, nil
}

// func (p *MetricAnomaly) ListClusters(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
// 	// For the UI to list clusters. Returns cluster_id

// 	ctxca, cancel := context.WithTimeout(ctx, timeout)
// 	defer cancel()
// 	// make the http request with context
// 	url := p.MetricAnomalyServiceURL + "/get_users"
// 	req, err := http.NewRequestWithContext(ctxca, http.MethodGet, url, nil)
// 	if err != nil {
// 		return nil, status.Errorf(codes.Internal, "Failed to form httprequest to ListClusters for metricAI: %s", err)
// 	}
// 	resp, err := p.httpClient.Do(req)
// 	if err != nil {
// 		return nil, status.Errorf(codes.Internal, "Failed to make httprequest to ListClusters for metricAI: %s", err)
// 	}
// 	defer resp.Body.Close()

// 	var result []string
// 	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
// 		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of ListClusters for metricAI: %s", err)
// 	}
// 	return &metricai.MetricAIIdList{Items: result}, nil
// }

func (p *MetricAnomaly) ListNamespaces(ctx context.Context, clusterId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
	// For the UI to list namespaces of a given cluster. Returns a list of namespaces

	// TODO: maybe GetSeriesMetrics or GetMetricLabelSets would work better here?
	response, err := p.cortexadminClient.Get().Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId.Id},
		Query:   "kube_namespace_labels",
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "Failed to query cortex: %s", err)
	}

	queryResult, err := compat.UnmarshalPrometheusResponse(response.Data)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal prometheus response: %s", err)
	}
	vec, err := queryResult.GetVector()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal prometheus response: %s", err)
	}
	list := &metricai.MetricAIIdList{}
	for _, sample := range *vec {
		list.Items = append(list.Items, string(sample.Metric[model.LabelName("namespace")]))
	}
	slices.Sort(list.Items)
	return list, nil
}

// list keys in the natsKV Job bucket
func (p *MetricAnomaly) ListJobs(ctx context.Context, _ *emptypb.Empty) (*metricai.MetricAIIdList, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobs for metricAI: %s", err)
	}
	jobs, err := metricAIKeyValue.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return &metricai.MetricAIIdList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to ListJobs for metricAI: %s", err)
	}
	return &metricai.MetricAIIdList{Items: jobs}, nil
}

// list keys in the natsKV JobRun bucket
func (p *MetricAnomaly) ListJobRuns(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIIdList, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobRuns for metricAI: %s", err)
	}
	jobruns, err := metricAIKeyValue.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return &metricai.MetricAIIdList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to ListJobRuns for metricAI: %s", err)
	}
	var jobRunIdArray []string
	// use jobId.Id + jobRunDelimiter to uniquely identify jobrun IDs for different jobs.
	for _, j := range jobruns {
		if strings.HasPrefix(j, jobId.Id+jobRunDelimiter) {
			jobRunIdArray = append(jobRunIdArray, j)
		}

	}
	return &metricai.MetricAIIdList{Items: jobRunIdArray}, nil
}

func (p *MetricAnomaly) RunJob(ctx context.Context, jobRequest *metricai.MetricAIId) (*metricai.MetricAIRunJobResponse, error) {
	// run a job. Post a request to the python metric-ai service.
	ctxca, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// make the http request with context
	url := p.MetricAnomalyServiceURL + "/run_job/" + jobRequest.Id
	req, err := http.NewRequestWithContext(ctxca, http.MethodGet, url, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to form httprequest to RunJob for metricAI: %s", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to make httprequest to RunJob for metricAI: %s", err)
	}
	defer resp.Body.Close()
	var res map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal response of SubmitJobRequest for metricAI: %s", err)
	}
	return &metricai.MetricAIRunJobResponse{JobRunId: res["JobRunId"].(string), SubmittedTime: time.Now().String(), Status: res["Status"].(string)}, nil
}

func (p *MetricAnomaly) JobIdValidate(jobId string) error {
	if jobId == "" { // id can't be empty
		return validation.Error("jobId can't be empty")
	}

	if strings.Contains(jobId, jobRunDelimiter) { // disallow the delimiter. TODO: should only allow chars include alphanum and - and _
		return validation.Error(fmt.Sprintf("jobId can't contain special char %s", jobRunDelimiter))
	}
	return nil
}

func (p *MetricAnomaly) CreateJob(ctx context.Context, jobRequest *metricai.MetricAICreateJobRequest) (*metricai.MetricAIAPIResponse, error) {
	// use the info provided by user to create a job
	// Info includes: job's name, the cluseter_id, a list of namespaces to watch.
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to ListJobRuns for metricAI: %s", err)
	}
	jid := strings.ToLower(jobRequest.JobId)
	err = p.JobIdValidate(jid)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to CreateJob with jobID %s for metricAI, Error: %s", jid, err)
	}

	if _, err := metricAIKeyValue.Get(jid); err == nil { // check if this id exists
		return nil, status.Errorf(codes.Internal, "Failed to CreateJob with jobID %s for metricAI, Error: The jobId to add already exist", jid)
	}

	job := make(map[string]interface{})
	job["JobId"] = jid
	job["JobCreateTime"] = time.Now().String()
	job["ClusterId"] = jobRequest.ClusterId
	job["Namespaces"] = jobRequest.Namespaces
	job["JobDescription"] = jobRequest.JobDescription
	jobJsonStr, err := json.Marshal(job)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal JobStatus for metricAI: %s", err)
	}
	_, err = metricAIKeyValue.Put(jid, []byte(jobJsonStr))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to CreateJob for metricAI: %s", err)
	}
	return &metricai.MetricAIAPIResponse{SubmittedTime: time.Now().String(), Status: "Success"}, nil
}

// delete job_id from the natsKV bucket. This won't delete the job_run attached to this job_id
func (p *MetricAnomaly) DeleteJob(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJob for metricAI: %s", err)
	}
	jid := jobId.Id
	if _, err := metricAIKeyValue.Get(jid); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, status.Errorf(codes.Internal, "Failed to DeleteJob for metricAI: The jobId to delete doesn't exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJob for metricAI: %s", err)
	}
	metricAIKeyValue.Delete(jid)
	return &metricai.MetricAIAPIResponse{Status: "Success", Description: fmt.Sprintf("The JobId key %s is deleted", jid)}, nil
}

// delete a job_run of a job. Won't delete the job itself.
func (p *MetricAnomaly) DeleteJobRun(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIAPIResponse, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobRun for metricAI: %s", err)
	}
	jid := jobRunId.Id
	if _, err := metricAIKeyValue.Get(jid); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, status.Errorf(codes.Internal, "Failed to DeleteJobRun for metricAI: The jobRunId to delete doesn't exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to DeleteJobRun for metricAI: %s", err)
	}
	metricAIKeyValue.Delete(jid)
	return &metricai.MetricAIAPIResponse{Status: "Success", Description: fmt.Sprintf("The JobRunId key :%s is deleted", jid)}, nil
}

// Grab the result of a job run from natsKV
func (p *MetricAnomaly) GetJobRunResult(ctx context.Context, jobRunId *metricai.MetricAIId) (*metricai.MetricAIJobRunResult, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIRunKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to GetJobRunResult bucket for metricAI: %s", err)
	}
	jid := jobRunId.Id
	jobRes, err := metricAIKeyValue.Get(jid)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, status.Errorf(codes.Internal, "Failed to GetJobRunResult with ID %s for metricAI: The jobRunId doesn't exist", jid)
		}
		return nil, status.Errorf(codes.Internal, "Failed to GetJobRunResult key %s for metricAI: %s", jid, err)
	}
	var res = metricai.MetricAIJobRunResult{}
	if err := json.Unmarshal(jobRes.Value(), &res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal GetJobRunResult from Jetstream for metricAI: %s", err)
	}
	return &res, nil

}

// Get the metadata from natsKV
func (p *MetricAnomaly) GetJob(ctx context.Context, jobId *metricai.MetricAIId) (*metricai.MetricAIJobStatus, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metricAIKeyValue, err := p.metricAIJobKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to GetJob for metricAI: %s", err)
	}
	jid := jobId.Id
	jobRes, err := metricAIKeyValue.Get(jid)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, status.Errorf(codes.Internal, "Failed to GetJob for metricAI: The jobId doesn't exist")
		}
		return nil, status.Errorf(codes.Internal, "Failed to GetJob for metricAI: %s", err)
	}
	var res = metricai.MetricAIJobStatus{}
	if err := json.Unmarshal(jobRes.Value(), &res); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal JobStatus from Jetstream for metricAI: %s", err)
	}
	return &res, nil

}
