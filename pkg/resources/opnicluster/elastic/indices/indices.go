package indices

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/elastic/go-elasticsearch/v7/esutil"
	"github.com/rancher/opni/apis/v1beta1"
	. "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	esClient ClientWithISM
	client   client.Client
	cluster  *v1beta1.OpniCluster
	ctx      context.Context
}

func NewReconciler(opniCluster *v1beta1.OpniCluster, ctx context.Context, client client.Client) *Reconciler {
	cfg := elasticsearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://opni-es-client.%s:9200", opniCluster.Namespace),
		},
		Username: "admin",
		Password: "admin",
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	esClient, _ := elasticsearch.NewClient(cfg)
	esExtendedclient := ClientWithISM{Client: esClient, ISM: &ISMApi{Client: esClient}}

	return &Reconciler{
		cluster:  opniCluster,
		esClient: esExtendedclient,
		ctx:      ctx,
		client:   client,
	}
}

func (r *Reconciler) indexExists(name string) (bool, error) {
	req := esapi.CatIndicesRequest{
		Index: []string{
			name,
		},
		Format: "json",
	}
	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to cat index %s: %s", name, resp.String())
	}

	return true, nil
}

func (r *Reconciler) shouldCreateTemplate(name string) (bool, error) {
	req := esapi.IndicesGetIndexTemplateRequest{
		Name: []string{
			name,
		},
	}
	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to check index template %s: %s", name, resp.String())
	}

	return false, nil
}

func (r *Reconciler) shouldBootstrapIndex(prefix string) (bool, error) {
	req := esapi.CatIndicesRequest{
		Index: []string{
			fmt.Sprintf("%s*", prefix),
		},
		Format: "json",
	}
	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return false, fmt.Errorf("checking indices failed: %s", resp.String())
	}

	indices := []map[string]interface{}{}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	err = json.Unmarshal(data, &indices)
	if err != nil {
		return false, err
	}

	return len(indices) == 0, nil
}

func (r *Reconciler) checkISMPolicy(policy *ISMPolicySpec) (bool, bool, int, int, error) {
	r.esClient.ISM.Policy = policy
	resp, err := r.esClient.ISM.GetISM(r.ctx)
	if err != nil {
		return false, false, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, false, 0, 0, nil
	} else if resp.IsError() {
		return false, false, 0, 0, fmt.Errorf("response from API is %s", resp.Status())
	}
	ismResponse := &ISMGetResponse{}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, false, 0, 0, err
	}
	json.Unmarshal(data, ismResponse)
	if reflect.DeepEqual(ismResponse.Policy, *policy) {
		return false, false, 0, 0, nil
	}
	return false, true, ismResponse.SeqNo, ismResponse.PrimaryTerm, nil
}

func (r *Reconciler) reconcileISM(policy *ISMPolicySpec) error {
	lg := log.FromContext(r.ctx)
	createIsm, updateIsm, seqNo, primaryTerm, err := r.checkISMPolicy(policy)
	if err != nil {
		return err
	}

	if createIsm {
		lg.Info("creating ism", "policy", policy.PolicyId)
		r.esClient.ISM.Policy = policy
		resp, err := r.esClient.ISM.CreateISM(r.ctx)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to create ism policy: %s", resp.String())
		}
		return nil
	}

	if updateIsm {
		lg.Info("updating existing ism", "policy", policy.PolicyId)
		r.esClient.ISM.Policy = policy
		resp, err := r.esClient.ISM.UpdateISM(r.ctx, seqNo, primaryTerm)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to update ism policy: %s", resp.String())
		}
		return nil
	}
	lg.V(1).Info("ism in sync", "policy", policy.PolicyId)
	return nil
}

func (r *Reconciler) maybeCreateIndexTemplate(template *IndexTemplateSpec) error {
	createTemplate, err := r.shouldCreateTemplate(template.TemplateName)
	if err != nil {
		return err
	}

	if createTemplate {
		req := esapi.IndicesPutIndexTemplateRequest{
			Body:   esutil.NewJSONReader(template),
			Name:   template.TemplateName,
			Create: esapi.BoolPtr(true),
		}
		resp, err := req.Do(r.ctx, r.esClient)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to create template %s: %s", template.TemplateName, resp.String())
		}
	}

	return nil
}

func (r *Reconciler) maybeBootstrapIndex(prefix string, alias string) error {
	bootstrap, err := r.shouldBootstrapIndex(prefix)
	if err != nil {
		return err
	}
	if bootstrap {
		indexReq := esapi.IndicesCreateRequest{
			Index: fmt.Sprintf("%s-000001", prefix),
		}
		indexResp, err := indexReq.Do(r.ctx, r.esClient)
		if err != nil {
			return err
		}
		defer indexResp.Body.Close()
		if indexResp.IsError() {
			return fmt.Errorf("failed to bootstrap %s: %s", prefix, indexResp.String())
		}
		aliasRequestBody := UpdateAliasRequest{
			Actions: []AliasActionSpec{
				{
					AliasAtomicAction: &AliasAtomicAction{
						Add: &AliasGenericAction{
							Index:        fmt.Sprintf("%s-000001", prefix),
							Alias:        alias,
							IsWriteIndex: esapi.BoolPtr(true),
						},
					},
				},
			},
		}
		aliasReq := esapi.IndicesUpdateAliasesRequest{
			Body: esutil.NewJSONReader(aliasRequestBody),
		}

		aliasResp, err := aliasReq.Do(r.ctx, r.esClient)
		if err != nil {
			return err
		}
		defer aliasResp.Body.Close()
		if aliasResp.IsError() {
			return fmt.Errorf("failed to update alias %s: %s", alias, aliasResp.String())
		}
	}

	return nil
}

func (r *Reconciler) maybeCreateIndex(name string, settings map[string]TemplateMappingsSpec) error {
	indexExists, err := r.indexExists(name)
	if err != nil {
		return err
	}

	if !indexExists {
		indexReq := esapi.IndicesCreateRequest{
			Index: name,
			Body:  esutil.NewJSONReader(settings),
		}
		resp, err := indexReq.Do(r.ctx, r.esClient)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to create index %s: %s", name, resp.String())
		}
	}

	return nil
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retError error) {
	kibanaDeployment := &appsv1.Deployment{}
	lg := log.FromContext(r.ctx)
	lg.V(1).Info("reconciling elastic indices")
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      "opni-es-kibana",
		Namespace: r.cluster.Namespace,
	}, kibanaDeployment)
	if err != nil {
		return nil, err
	}

	if kibanaDeployment.Status.AvailableReplicas < 1 {
		lg.Info("waiting for elastic stack")
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.cluster), r.cluster); err != nil {
				return err
			}
			r.cluster.Status.Conditions = append(r.cluster.Status.Conditions, "waiting for elastic cluster to be available")
			return r.client.Status().Update(r.ctx, r.cluster)
		})

		return &reconcile.Result{RequeueAfter: 5 * time.Second}, err
	}

	for _, policy := range []ISMPolicySpec{
		opniLogPolicy,
		opniDrainModelStatusPolicy,
	} {
		err = r.reconcileISM(&policy)
		if err != nil {

			return nil, err
		}
	}

	for _, template := range []IndexTemplateSpec{
		opniLogTemplate,
		drainStatusTemplate,
	} {
		err = r.maybeCreateIndexTemplate(&template)
		if err != nil {
			return nil, err
		}
	}

	for prefix, alias := range map[string]string{
		logIndexPrefix:         logIndexAlias,
		drainStatusIndexPrefix: drainStatusIndexAlias,
	} {
		err = r.maybeBootstrapIndex(prefix, alias)
		if err != nil {
			return nil, err
		}
	}

	err = r.maybeCreateIndex(normalIntervalIndexName, normalIntervalIndexSettings)
	if err != nil {
		return nil, err
	}

	return
}
