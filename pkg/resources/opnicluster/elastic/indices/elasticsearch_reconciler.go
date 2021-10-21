package indices

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opensearchapiext "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
	"github.com/rancher/opni/pkg/util/kibana"
	"golang.org/x/mod/semver"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type elasticsearchReconciler struct {
	esClient     ExtendedClient
	kibanaClient *kibana.Client
	ctx          context.Context
}

func newElasticsearchReconciler(ctx context.Context, namespace string, password string) *elasticsearchReconciler {
	esCfg := opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://opni-es-client.%s:9200", namespace),
		},
		Username:             "admin",
		Password:             password,
		UseResponseCheckOnly: true,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	kbCfg := kibana.Config{
		URL:      fmt.Sprintf("http://opni-es-kibana.%s:5601", namespace),
		Username: "admin",
		Password: password,
	}
	kbClient, _ := kibana.NewClient(kbCfg)
	esClient, _ := opensearch.NewClient(esCfg)
	esExtendedclient := ExtendedClient{
		Client: esClient,
		ISM:    &ISMApi{Client: esClient},
	}
	return &elasticsearchReconciler{
		esClient:     esExtendedclient,
		kibanaClient: kbClient,
		ctx:          ctx,
	}
}

func newElasticsearchReconcilerWithTransport(ctx context.Context, namespace string, transport http.RoundTripper) *elasticsearchReconciler {
	esCfg := opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://opni-es-client.%s:9200", namespace),
		},
		Username:             "admin",
		Password:             "admin",
		UseResponseCheckOnly: true,
		Transport:            transport,
	}
	kbCfg := kibana.Config{
		URL:       fmt.Sprintf("http://opni-es-kibana.%s:5601", namespace),
		Username:  "admin",
		Password:  "admin",
		Transport: transport,
	}
	kbClient, _ := kibana.NewClient(kbCfg)
	esClient, _ := opensearch.NewClient(esCfg)
	esExtendedclient := ExtendedClient{
		Client: esClient,
		ISM:    &ISMApi{Client: esClient},
	}
	return &elasticsearchReconciler{
		esClient:     esExtendedclient,
		kibanaClient: kbClient,
		ctx:          ctx,
	}
}

func (r *elasticsearchReconciler) indexExists(name string) (bool, error) {
	req := opensearchapi.CatIndicesRequest{
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

func (r *elasticsearchReconciler) shouldCreateTemplate(name string) (bool, error) {
	req := opensearchapi.IndicesGetIndexTemplateRequest{
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

func (r *elasticsearchReconciler) shouldBootstrapIndex(prefix string) (bool, error) {
	req := opensearchapi.CatIndicesRequest{
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
	err = json.NewDecoder(resp.Body).Decode(&indices)
	if err != nil {
		return false, err
	}

	return len(indices) == 0, nil
}

func (r *elasticsearchReconciler) checkISMPolicy(policy *opensearchapiext.ISMPolicySpec) (bool, bool, int, int, error) {
	resp, err := r.esClient.ISM.GetISM(r.ctx, policy.PolicyID)
	if err != nil {
		return false, false, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, false, 0, 0, nil
	} else if resp.IsError() {
		return false, false, 0, 0, fmt.Errorf("response from API is %s", resp.Status())
	}
	ismResponse := &opensearchapiext.ISMGetResponse{}
	err = json.NewDecoder(resp.Body).Decode(ismResponse)
	if err != nil {
		return false, false, 0, 0, err
	}
	if reflect.DeepEqual(ismResponse.Policy, *policy) {
		return false, false, 0, 0, nil
	}
	return false, true, ismResponse.SeqNo, ismResponse.PrimaryTerm, nil
}

func (r *elasticsearchReconciler) reconcileISM(policy *opensearchapiext.ISMPolicySpec) error {
	lg := log.FromContext(r.ctx)
	policyBody := map[string]interface{}{
		"policy": policy,
	}
	createIsm, updateIsm, seqNo, primaryTerm, err := r.checkISMPolicy(policy)
	if err != nil {
		return err
	}

	if createIsm {
		lg.Info("creating ism", "policy", policy.PolicyID)
		resp, err := r.esClient.ISM.CreateISM(r.ctx, policy.PolicyID, opensearchutil.NewJSONReader(policyBody))
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
		lg.Info("updating existing ism", "policy", policy.PolicyID)
		resp, err := r.esClient.ISM.UpdateISM(r.ctx, policy.PolicyID, opensearchutil.NewJSONReader(policyBody), seqNo, primaryTerm)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to update ism policy: %s", resp.String())
		}
		return nil
	}
	lg.V(1).Info("ism in sync", "policy", policy.PolicyID)
	return nil
}

func (r *elasticsearchReconciler) maybeCreateIndexTemplate(template *opensearchapiext.IndexTemplateSpec) error {
	createTemplate, err := r.shouldCreateTemplate(template.TemplateName)
	if err != nil {
		return err
	}

	if createTemplate {
		req := opensearchapi.IndicesPutIndexTemplateRequest{
			Body:   opensearchutil.NewJSONReader(template),
			Name:   template.TemplateName,
			Create: opensearchapi.BoolPtr(true),
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

func (r *elasticsearchReconciler) maybeBootstrapIndex(prefix string, alias string) error {
	bootstrap, err := r.shouldBootstrapIndex(prefix)
	lg := log.FromContext(r.ctx)
	if err != nil {
		return err
	}
	if bootstrap {
		indexReq := opensearchapi.IndicesCreateRequest{
			Index: fmt.Sprintf("%s-000001", prefix),
		}
		lg.Info(fmt.Sprintf("creating index %s-000001", prefix))
		indexResp, err := indexReq.Do(r.ctx, r.esClient)
		if err != nil {
			return err
		}
		defer indexResp.Body.Close()
		if indexResp.IsError() {
			return fmt.Errorf("failed to bootstrap %s: %s", prefix, indexResp.String())
		}

		aliasIsIndex, err := r.indexExists(alias)
		if err != nil {
			return err
		}
		aliasRequestBody := opensearchapiext.UpdateAliasRequest{
			Actions: []opensearchapiext.AliasActionSpec{
				{
					AliasAtomicAction: &opensearchapiext.AliasAtomicAction{
						Add: &opensearchapiext.AliasGenericAction{
							Index:        fmt.Sprintf("%s-000001", prefix),
							Alias:        alias,
							IsWriteIndex: opensearchapi.BoolPtr(true),
						},
					},
				},
			},
		}

		if aliasIsIndex {
			body := opensearchapiext.ReindexSpec{
				Source: opensearchapiext.ReindexIndexSpec{
					Index: alias,
				},
				Destination: opensearchapiext.ReindexIndexSpec{
					Index: fmt.Sprintf("%s-000001", prefix),
				},
			}
			reindexReq := opensearchapi.ReindexRequest{
				Body:              opensearchutil.NewJSONReader(body),
				WaitForCompletion: opensearchapi.BoolPtr(true),
			}
			lg.Info(fmt.Sprintf("reindexing %s into %s-000001", alias, prefix))
			reindexResp, err := reindexReq.Do(r.ctx, r.esClient)
			if err != nil {
				return err
			}
			defer reindexResp.Body.Close()
			if reindexResp.IsError() {
				return fmt.Errorf("failed to reindex %s: %s", alias, reindexResp.String())
			}

			aliasRequestBody.Actions = append(aliasRequestBody.Actions, opensearchapiext.AliasActionSpec{
				AliasAtomicAction: &opensearchapiext.AliasAtomicAction{
					RemoveIndex: &opensearchapiext.AliasGenericAction{
						Index: alias,
					},
				},
			})
		}

		aliasReq := opensearchapi.IndicesUpdateAliasesRequest{
			Body: opensearchutil.NewJSONReader(aliasRequestBody),
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

func (r *elasticsearchReconciler) maybeCreateIndex(name string, settings map[string]opensearchapiext.TemplateMappingsSpec) error {
	indexExists, err := r.indexExists(name)
	if err != nil {
		return err
	}

	if !indexExists {
		indexReq := opensearchapi.IndicesCreateRequest{
			Index: name,
			Body:  opensearchutil.NewJSONReader(settings),
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

func (r *elasticsearchReconciler) shouldUpdateKibana() (retBool bool, retErr error) {
	exists, err := r.indexExists(kibanaDashboardVersionIndex)
	if err != nil {
		return false, err
	}

	if !exists {
		return true, nil
	}

	respDoc := &opensearchapiext.KibanaDocResponse{}
	req := opensearchapi.GetRequest{
		Index:      kibanaDashboardVersionIndex,
		DocumentID: kibanaDashboardVersionDocID,
	}

	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to check kibana version doc: %s", resp.String())
	}

	err = json.NewDecoder(resp.Body).Decode(respDoc)
	if err != nil {
		return false, err
	}

	if semver.Compare(respDoc.Source.DashboardVersion, kibanaDashboardVersion) < 0 {
		return true, nil
	}

	return
}

func (r *elasticsearchReconciler) upsertKibanaObjectDoc() error {
	upsertRequest := opensearchapiext.UpsertKibanaDoc{
		Document:         kibanaDoc,
		DocumentAsUpsert: opensearchapi.BoolPtr(true),
	}

	req := opensearchapi.UpdateRequest{
		Index:      kibanaDashboardVersionIndex,
		DocumentID: kibanaDashboardVersionDocID,
		Body:       opensearchutil.NewJSONReader(upsertRequest),
	}
	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to upsert kibana doc: %s", resp.String())
	}

	return nil
}

func (r *elasticsearchReconciler) importKibanaObjects() error {
	lg := log.FromContext(r.ctx)
	update, err := r.shouldUpdateKibana()
	if err != nil {
		return err
	}

	if update {
		lg.Info("updating kibana saved objects")
		resp, err := r.kibanaClient.ImportObjects(r.ctx, kibanaObjects, "objects.ndjson")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("unable to import kibana objects: %s", resp.String())
		}

		return r.upsertKibanaObjectDoc()
	}

	lg.V(1).Info("kibana objects on latest version")
	return nil
}
