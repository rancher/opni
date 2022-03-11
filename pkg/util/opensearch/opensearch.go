package opensearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/util/kibana"
	opensearchapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"golang.org/x/mod/semver"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	headerContentType        = "Content-Type"
	kibanaCrossHeaderType    = "kbn-xsrf"
	securityTenantHeaderType = "securitytenant"

	jsonContentHeader = "application/json"

	ISMChangeVersion = "1.1.0"
)

type Reconciler struct {
	osClient     ExtendedClient
	kibanaClient *kibana.Client
	ctx          context.Context
}

type ExtendedClient struct {
	*opensearch.Client
	ISM      *ISMApi
	Security *SecurityAPI
}

func NewReconciler(ctx context.Context, namespace string, password string, osServiceName string, kbServiceName string) *Reconciler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	esCfg := opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://%s.%s:9200", osServiceName, namespace),
		},
		Username:             "admin",
		Password:             password,
		UseResponseCheckOnly: true,
		Transport:            transport,
	}
	kbCfg := kibana.Config{
		URL:      fmt.Sprintf("http://%s.%s:5601", kbServiceName, namespace),
		Username: "admin",
		Password: password,
	}
	kbClient, _ := kibana.NewClient(kbCfg)
	esClient, _ := opensearch.NewClient(esCfg)
	esExtendedclient := ExtendedClient{
		Client:   esClient,
		ISM:      &ISMApi{Client: esClient},
		Security: &SecurityAPI{Client: esClient},
	}
	return &Reconciler{
		osClient:     esExtendedclient,
		kibanaClient: kbClient,
		ctx:          ctx,
	}
}

func newElasticsearchReconcilerWithTransport(ctx context.Context, namespace string, transport http.RoundTripper) *Reconciler {
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
	return &Reconciler{
		osClient:     esExtendedclient,
		kibanaClient: kbClient,
		ctx:          ctx,
	}
}

func (r *Reconciler) indexExists(name string) (bool, error) {
	req := opensearchapi.CatIndicesRequest{
		Index: []string{
			name,
		},
		Format: "json",
	}
	resp, err := req.Do(r.ctx, r.osClient)
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
	req := opensearchapi.IndicesGetIndexTemplateRequest{
		Name: []string{
			name,
		},
	}
	resp, err := req.Do(r.ctx, r.osClient)
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
	req := opensearchapi.CatIndicesRequest{
		Index: []string{
			fmt.Sprintf("%s*", prefix),
		},
		Format: "json",
	}
	resp, err := req.Do(r.ctx, r.osClient)
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

func (r *Reconciler) checkISMPolicy(policy interface{}) (bool, bool, int, int, error) {
	policyID := reflect.ValueOf(policy).FieldByName("PolicyID").String()
	resp, err := r.osClient.ISM.GetISM(r.ctx, policyID)
	if err != nil {
		return false, false, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, false, 0, 0, nil
	} else if resp.IsError() {
		return false, false, 0, 0, fmt.Errorf("response from API is %s", resp.Status())
	}
	switch ism := policy.(type) {
	case opensearchapiext.OldISMPolicySpec:
		ismResponse := &opensearchapiext.OldISMGetResponse{}
		err = json.NewDecoder(resp.Body).Decode(ismResponse)
		if err != nil {
			return false, false, 0, 0, err
		}
		if reflect.DeepEqual(ismResponse.Policy, ism) {
			return false, false, 0, 0, nil
		}
		return false, true, ismResponse.SeqNo, ismResponse.PrimaryTerm, nil
	case opensearchapiext.ISMPolicySpec:
		ismResponse := &opensearchapiext.ISMGetResponse{}
		err = json.NewDecoder(resp.Body).Decode(ismResponse)
		if err != nil {
			return false, false, 0, 0, err
		}
		if reflect.DeepEqual(ismResponse.Policy, ism) {
			return false, false, 0, 0, nil
		}
		return false, true, ismResponse.SeqNo, ismResponse.PrimaryTerm, nil
	default:
		return false, false, 0, 0, errors.New("invalid ISM policy type")
	}
}

func (r *Reconciler) ReconcileISM(policy interface{}) error {
	lg := log.FromContext(r.ctx)
	policyID := reflect.ValueOf(policy).FieldByName("PolicyID").String()
	policyBody := map[string]interface{}{
		"policy": policy,
	}
	createIsm, updateIsm, seqNo, primaryTerm, err := r.checkISMPolicy(policy)
	if err != nil {
		return err
	}

	if createIsm {
		lg.Info("creating ism", "policy", policyID)
		resp, err := r.osClient.ISM.CreateISM(r.ctx, policyID, opensearchutil.NewJSONReader(policyBody))
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
		lg.Info("updating existing ism", "policy", policyID)
		resp, err := r.osClient.ISM.UpdateISM(r.ctx, policyID, opensearchutil.NewJSONReader(policyBody), seqNo, primaryTerm)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("failed to update ism policy: %s", resp.String())
		}
		return nil
	}
	lg.V(1).Info("ism in sync", "policy", policyID)
	return nil
}

func (r *Reconciler) MaybeCreateIndexTemplate(template *opensearchapiext.IndexTemplateSpec) error {
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
		resp, err := req.Do(r.ctx, r.osClient)
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

func (r *Reconciler) MaybeBootstrapIndex(prefix string, alias string) error {
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
		indexResp, err := indexReq.Do(r.ctx, r.osClient)
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
			reindexResp, err := reindexReq.Do(r.ctx, r.osClient)
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

		aliasResp, err := aliasReq.Do(r.ctx, r.osClient)
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

func (r *Reconciler) MaybeCreateIndex(name string, settings map[string]opensearchapiext.TemplateMappingsSpec) error {
	indexExists, err := r.indexExists(name)
	if err != nil {
		return err
	}

	if !indexExists {
		indexReq := opensearchapi.IndicesCreateRequest{
			Index: name,
			Body:  opensearchutil.NewJSONReader(settings),
		}
		resp, err := indexReq.Do(r.ctx, r.osClient)
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

func (r *Reconciler) shouldUpdateKibana(indexName string, docID string, version string) (retBool bool, retErr error) {
	exists, err := r.indexExists(indexName)
	if err != nil {
		return false, err
	}

	if !exists {
		return true, nil
	}

	respDoc := &opensearchapiext.KibanaDocResponse{}
	req := opensearchapi.GetRequest{
		Index:      indexName,
		DocumentID: docID,
	}

	resp, err := req.Do(r.ctx, r.osClient)
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

	if semver.Compare(respDoc.Source.DashboardVersion, version) < 0 {
		return true, nil
	}

	return
}

func (r *Reconciler) upsertKibanaObjectDoc(indexName string, docID string, version string) error {
	kibanaDoc := opensearchapiext.KibanaVersionDoc{
		DashboardVersion: version,
	}

	upsertRequest := opensearchapiext.UpsertKibanaDoc{
		Document:         kibanaDoc,
		DocumentAsUpsert: opensearchapi.BoolPtr(true),
	}

	req := opensearchapi.UpdateRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       opensearchutil.NewJSONReader(upsertRequest),
	}
	resp, err := req.Do(r.ctx, r.osClient)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to upsert kibana doc: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) ImportKibanaObjects(indexName string, docID string, version string, kibanaObjects string) error {
	lg := log.FromContext(r.ctx)
	update, err := r.shouldUpdateKibana(indexName, docID, version)
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

		return r.upsertKibanaObjectDoc(indexName, docID, version)
	}

	lg.V(1).Info("kibana objects on latest version")
	return nil
}

func (r *Reconciler) shouldCreateRole(name string) (bool, error) {
	resp, err := r.osClient.Security.GetRole(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.Status())
	}

	return false, nil
}

func (r *Reconciler) MaybeCreateRole(role opensearchapiext.RoleSpec) error {
	createRole, err := r.shouldCreateRole(role.RoleName)
	if err != nil {
		return err
	}
	if !createRole {
		return nil
	}

	resp, err := r.osClient.Security.CreateRole(r.ctx, role.RoleName, opensearchutil.NewJSONReader(role))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to create role: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) shouldCreateUser(name string) (bool, error) {
	resp, err := r.osClient.Security.GetUser(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.Status())
	}

	return false, nil
}

func (r *Reconciler) MaybeCreateUser(user opensearchapiext.UserSpec) error {
	createRole, err := r.shouldCreateUser(user.UserName)
	if err != nil {
		return err
	}
	if !createRole {
		return nil
	}

	resp, err := r.osClient.Security.CreateUser(r.ctx, user.UserName, opensearchutil.NewJSONReader(user))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to create user: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) checkRolesMapping(roleName string, userName string) (bool, opensearchapiext.RoleMappingSpec, error) {
	mapping := opensearchapiext.RoleMappingSpec{}

	resp, err := r.osClient.Security.GetRolesMapping(r.ctx, roleName)

	if err != nil {
		return false, mapping, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		mapping.Users = append(mapping.Users, userName)
		return true, mapping, nil
	}

	mappingResp := opensearchapiext.RoleMappingReponse{}
	json.NewDecoder(resp.Body).Decode(&mappingResp)
	mapping = mappingResp[roleName]
	for _, user := range mapping.Users {
		if user == userName {
			return false, mapping, nil
		}
	}
	mapping.Users = append(mapping.Users, userName)
	return true, mapping, nil
}

func (r *Reconciler) MaybeUpdateRolesMapping(roleName string, userName string) error {
	shouldUpdate, mapping, err := r.checkRolesMapping(roleName, userName)
	if err != nil {
		return err
	}

	if !shouldUpdate {
		return nil
	}

	resp, err := r.osClient.Security.CreateRolesMapping(r.ctx, roleName, opensearchutil.NewJSONReader(mapping))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to create rolesmapping: %s", resp.String())
	}
	return nil
}
