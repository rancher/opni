package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/opensearch/dashboards"
	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/opensearch/opensearch/api"
	oserrors "github.com/rancher/opni/pkg/opensearch/opensearch/errors"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/mod/semver"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Reconciler struct {
	ReconcilerOptions
	ctx context.Context
}

type ReconcilerConfig struct {
	CertReader            certs.OpensearchCertReader
	OpensearchServiceName string
	DashboardsServiceName string
}

type ReconcilerOptions struct {
	osClient           *opensearch.Client
	dashboardsClient   *dashboards.Client
	dashboardsUsername string
	dashboardsPassword string
}

type ReconcilerOption func(*ReconcilerOptions)

func (o *ReconcilerOptions) apply(opts ...ReconcilerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithOpensearchClient(client *opensearch.Client) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.osClient = client
	}
}

func WithDashboardshClient(client *dashboards.Client) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.dashboardsClient = client
	}
}

func WithDashboardsPassword(password string) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.dashboardsPassword = password
	}
}

func WithDashboardsUsername(username string) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.dashboardsUsername = username
	}
}

func NewReconciler(
	ctx context.Context,
	cfg ReconcilerConfig,
	opts ...ReconcilerOption,
) (*Reconciler, error) {
	options := ReconcilerOptions{}
	options.apply(opts...)

	if options.osClient == nil {
		oscfg := opensearch.ClientConfig{
			URLs: []string{
				fmt.Sprintf("https://%s:9200", cfg.OpensearchServiceName),
			},
			CertReader: cfg.CertReader,
		}
		osClient, err := opensearch.NewClient(oscfg)
		if err != nil {
			return nil, err
		}
		options.osClient = osClient
	}

	if options.dashboardsClient == nil {
		dashboardscfg := dashboards.Config{
			URL:      fmt.Sprintf("https://%s:5601", cfg.DashboardsServiceName),
			Username: options.dashboardsUsername,
			Password: options.dashboardsPassword,
		}
		dashboardsClient, err := dashboards.NewClient(dashboardscfg)
		if err != nil {
			return nil, err
		}
		options.dashboardsClient = dashboardsClient
	}

	return &Reconciler{
		ReconcilerOptions: options,
		ctx:               ctx,
	}, nil
}

func (r *Reconciler) indexExists(name string) (bool, error) {
	resp, err := r.osClient.Indices.CatIndices(r.ctx, []string{name})
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

func (r *Reconciler) shouldCreateTemplate(template types.IndexTemplateSpec) (bool, error) {
	lg := log.FromContext(r.ctx)

	resp, err := r.osClient.Indices.GetIndexTemplates(r.ctx, []string{template.TemplateName})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to check index template %s: %s", template.TemplateName, resp.String())
	}

	getTemplateResp := &types.GetIndexTemplateResponse{}
	err = json.NewDecoder(resp.Body).Decode(getTemplateResp)
	if err != nil {
		return false, err
	}

	for _, remoteTemplate := range getTemplateResp.IndexTemplates {
		if remoteTemplate.Name == template.TemplateName {
			compared := remoteTemplate.Template
			compared.TemplateName = remoteTemplate.Name
			if reflect.DeepEqual(compared, template) {
				return false, nil
			}
			lg.Info("template exists but is different, updating", "template", template.TemplateName)
			return true, nil
		}
	}

	return false, nil
}

func (r *Reconciler) TemplateExists(name string) (bool, error) {
	resp, err := r.osClient.Indices.GetIndexTemplates(r.ctx, []string{name})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to check index template %s: %s", name, resp.String())
	}

	return true, nil
}

func (r *Reconciler) prefixIndicesAbsent(prefix string) (bool, error) {
	names := []string{
		fmt.Sprintf("%s*", prefix),
	}
	resp, err := r.osClient.Indices.CatIndices(r.ctx, names)
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

func (r *Reconciler) checkISMPolicy(policy types.ISMPolicySpec) (bool, bool, int, int, error) {
	resp, err := r.osClient.ISM.GetISM(r.ctx, policy.PolicyID)
	if err != nil {
		return false, false, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return true, false, 0, 0, nil
	} else if resp.IsError() {
		return false, false, 0, 0, fmt.Errorf("response from API is %s", resp.String())
	}
	ismResponse := &types.ISMGetResponse{}
	err = json.NewDecoder(resp.Body).Decode(ismResponse)
	if err != nil {
		return false, false, 0, 0, err
	}
	if reflect.DeepEqual(ismResponse.Policy, policy) {
		return false, false, 0, 0, nil
	}
	return false, true, ismResponse.SeqNo, ismResponse.PrimaryTerm, nil
}

func (r *Reconciler) ReconcileISM(policy types.ISMPolicySpec) error {
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
		resp, err := r.osClient.ISM.CreateISM(r.ctx, policy.PolicyID, opensearchutil.NewJSONReader(policyBody))
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
		resp, err := r.osClient.ISM.UpdateISM(r.ctx, policy.PolicyID, opensearchutil.NewJSONReader(policyBody), seqNo, primaryTerm)
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

func (r *Reconciler) MaybeCreateIndexTemplate(template types.IndexTemplateSpec) error {
	createTemplate, err := r.shouldCreateTemplate(template)
	if err != nil {
		return err
	}

	if createTemplate {
		resp, err := r.osClient.Indices.PutIndexTemplate(r.ctx, template.TemplateName, opensearchutil.NewJSONReader(template))
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

func (r *Reconciler) MaybeDeleteIndexTemplate(name string) error {
	exists, err := r.TemplateExists(name)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	resp, err := r.osClient.Indices.DeleteIndexTemplate(r.ctx, name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to delete template %s: %s", name, resp.String())
	}

	return nil
}

func (r *Reconciler) MaybeBootstrapIndex(prefix string, alias string, oldPrefixes []string) error {
	bootstrap, err := r.prefixIndicesAbsent(prefix)
	lg := log.FromContext(r.ctx)
	if err != nil {
		return err
	}
	if bootstrap {
		name := fmt.Sprintf("%s-000001", prefix)
		lg.Info(fmt.Sprintf("creating index %s-000001", prefix))
		indexResp, err := r.osClient.Indices.CreateIndex(r.ctx, name, nil)
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

		var oldPrefixesExist bool
		if len(oldPrefixes) > 0 {
			oldPrefixesExist, err = r.oldIndicesExist(oldPrefixes)
			if err != nil {
				return err
			}
		}
		aliasRequestBody := types.UpdateAliasRequest{
			Actions: []types.AliasActionSpec{
				{
					AliasAtomicAction: &types.AliasAtomicAction{
						Add: &types.AliasGenericAction{
							Index:        fmt.Sprintf("%s-000001", prefix),
							Alias:        alias,
							IsWriteIndex: lo.ToPtr(true),
						},
					},
				},
			},
		}

		if oldPrefixesExist || !aliasIsIndex {
			if oldPrefixesExist {
				for _, prefix := range oldPrefixes {
					exists, _ := r.oldIndicesExist([]string{prefix})
					if exists {
						aliasRequestBody.Actions = append(aliasRequestBody.Actions, types.AliasActionSpec{
							AliasAtomicAction: &types.AliasAtomicAction{
								Remove: &types.AliasGenericAction{
									Index: prefix,
									Alias: alias,
								},
							},
						})
					}
				}
			}

			aliasResp, err := r.osClient.Indices.UpdateAlias(r.ctx, opensearchutil.NewJSONReader(aliasRequestBody))
			if err != nil {
				return err
			}
			defer aliasResp.Body.Close()
			if aliasResp.IsError() {
				return fmt.Errorf("failed to update alias %s: %s", alias, aliasResp.String())
			}
		}

		if oldPrefixesExist || aliasIsIndex {
			body := types.ReindexSpec{
				Source: types.ReindexSourceSpec{
					Index: func() []string {
						if oldPrefixesExist {
							return oldPrefixes
						}
						return []string{alias}
					}(),
				},
				Destination: types.ReindexDestSpec{
					Index: fmt.Sprintf("%s-000001", prefix),
				},
			}
			lg.Info(fmt.Sprintf("reindexing %s into %s-000001", alias, prefix))
			reindexResp, err := r.osClient.Indices.SynchronousReindex(r.ctx, opensearchutil.NewJSONReader(body))
			if err != nil {
				return err
			}
			defer reindexResp.Body.Close()
			if reindexResp.IsError() {
				return fmt.Errorf("failed to reindex %s: %s", alias, reindexResp.String())
			}
		}

		if oldPrefixesExist {
			deleteResp, err := r.osClient.Indices.DeleteIndices(r.ctx, oldPrefixes)
			if err != nil {
				return err
			}
			defer deleteResp.Body.Close()
			if deleteResp.IsError() {
				return fmt.Errorf("failed to delete old indices %s", deleteResp.String())
			}
		} else if aliasIsIndex {
			aliasRequestBody.Actions = append(aliasRequestBody.Actions, types.AliasActionSpec{
				AliasAtomicAction: &types.AliasAtomicAction{
					RemoveIndex: &types.AliasGenericAction{
						Index: alias,
					},
				},
			})
			aliasResp, err := r.osClient.Indices.UpdateAlias(r.ctx, opensearchutil.NewJSONReader(aliasRequestBody))
			if err != nil {
				return err
			}
			defer aliasResp.Body.Close()
			if aliasResp.IsError() {
				return fmt.Errorf("failed to update alias %s: %s", alias, aliasResp.String())
			}
		}

	}

	return nil
}

func (r *Reconciler) oldIndicesExist(oldPrexifes []string) (bool, error) {
	resp, err := r.osClient.Indices.CatIndices(r.ctx, oldPrexifes)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return false, fmt.Errorf("failed to cat indices %s", resp.String())
	}

	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	var indices []string
	gjson.Get(b.String(), "#.index").ForEach(func(key, value gjson.Result) bool {
		indices = append(indices, value.String())
		return true
	})

	return len(indices) > 0, nil
}

func (r *Reconciler) MaybeCreateIndex(name string, settings map[string]types.TemplateMappingsSpec) error {
	indexExists, err := r.indexExists(name)
	if err != nil {
		return err
	}

	if !indexExists {
		resp, err := r.osClient.Indices.CreateIndex(r.ctx, name, opensearchutil.NewJSONReader(settings))
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

func (r *Reconciler) shouldUpdateDashboards(indexName string, docID string, version string) (retBool bool, retErr error) {
	exists, err := r.indexExists(indexName)
	if err != nil {
		return false, err
	}

	if !exists {
		return true, nil
	}

	respDoc := &types.DashboardsDocResponse{}

	resp, err := r.osClient.Indices.GetDocument(r.ctx, indexName, docID)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("failed to check dashboards version doc: %s", resp.String())
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

func (r *Reconciler) upsertDashboardsObjectDoc(indexName string, docID string, version string) error {
	kibanaDoc := types.DashboardsVersionDoc{
		DashboardVersion: version,
	}

	upsertRequest := types.UpsertDashboardsDoc{
		Document:         kibanaDoc,
		DocumentAsUpsert: lo.ToPtr(true),
	}

	resp, err := r.osClient.Indices.UpdateDocument(r.ctx, indexName, docID, opensearchutil.NewJSONReader(upsertRequest))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to upsert kibana doc: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) ImportKibanaObjects(indexName string, docID string, version string, dashboardsObjects string) error {
	lg := log.FromContext(r.ctx)
	update, err := r.shouldUpdateDashboards(indexName, docID, version)
	if err != nil {
		return err
	}

	if update {
		lg.Info("updating kibana saved objects")
		resp, err := r.dashboardsClient.ImportObjects(r.ctx, dashboardsObjects, "objects.ndjson")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("unable to import kibana objects: %s", resp.String())
		}

		return r.upsertDashboardsObjectDoc(indexName, docID, version)
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
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return false, nil
}

func (r *Reconciler) MaybeCreateRole(role types.RoleSpec) error {
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

func (r *Reconciler) MaybeDeleteRole(rolename string) error {
	absent, err := r.shouldCreateRole(rolename)
	if err != nil {
		return err
	}
	if absent {
		return nil
	}

	resp, err := r.osClient.Security.DeleteRole(r.ctx, rolename)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to delete role: %s", resp.String())
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
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return false, nil
}

func (r *Reconciler) MaybeCreateUser(user types.UserSpec) error {
	createUser, err := r.shouldCreateUser(user.UserName)
	if err != nil {
		return err
	}
	if !createUser {
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

func (r *Reconciler) MaybeDeleteUser(username string) error {
	absent, err := r.shouldCreateUser(username)
	if err != nil {
		return err
	}
	if absent {
		return nil
	}

	resp, err := r.osClient.Security.DeleteUser(r.ctx, username)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to delete user: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) checkRolesMapping(roleName string, userName string) (bool, types.RoleMappingSpec, error) {
	mapping := types.RoleMappingSpec{}

	resp, err := r.osClient.Security.GetRolesMapping(r.ctx, roleName)

	if err != nil {
		return false, mapping, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		mapping.Users = append(mapping.Users, userName)
		return true, mapping, nil
	}

	mappingResp := types.RoleMappingReponse{}
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

func (r *Reconciler) MaybeRemoveRolesMapping(roleName string, userName string) error {
	userAbsent, mapping, err := r.checkRolesMapping(roleName, userName)
	if err != nil {
		return err
	}

	if userAbsent {
		return nil
	}

	var users []string
	for _, user := range mapping.Users {
		if user != userName {
			users = append(users, userName)
		}
	}

	var resp *api.Response
	if len(users) == 0 {
		resp, err = r.osClient.Security.DeleteRolesMapping(r.ctx, roleName)
	} else {
		// If there are still users we update the mapping
		mapping.Users = users
		resp, err = r.osClient.Security.CreateRolesMapping(r.ctx, roleName, opensearchutil.NewJSONReader(mapping))
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to remove user role mapping: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) shouldUpdateIngestPipeline(name string, new types.IngestPipeline) (bool, error) {
	lg := log.FromContext(r.ctx)

	resp, err := r.osClient.Ingest.GetIngestPipeline(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	existing := types.IngestPipeline{}
	err = json.NewDecoder(resp.Body).Decode(&existing)
	if err != nil {
		return false, err
	}

	if !reflect.DeepEqual(new, existing) {
		lg.Info("pipeline template exists but is different")
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) ingestPipelineExists(name string) (bool, error) {
	resp, err := r.osClient.Ingest.GetIngestPipeline(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return true, nil
}

func (r *Reconciler) MaybeCreateIngestPipeline(name string, pipeline types.IngestPipeline) error {
	shouldCreate, err := r.shouldUpdateIngestPipeline(name, pipeline)
	if err != nil {
		return err
	}

	if !shouldCreate {
		return nil
	}

	resp, err := r.osClient.Ingest.PutIngestTemplate(r.ctx, name, opensearchutil.NewJSONReader(pipeline))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to create ingest pipeline: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) MaybeDeleteIngestPipeline(name string) error {
	exists, err := r.ingestPipelineExists(name)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	resp, err := r.osClient.Ingest.DeleteIngestPipeline(r.ctx, name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to delete ingest pipeline: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) UpdateDefaultIngestPipelineForIndex(index string, pipelineName string) error {
	var absent bool
	var err error

	if strings.HasSuffix(index, "*") {
		absent, err = r.prefixIndicesAbsent(index)
		if err != nil {
			return err
		}
	} else {
		exists, err := r.indexExists(index)
		absent = !exists
		if err != nil {
			return err
		}
	}

	// If indices don't exist we don't need to update anything
	if absent {
		return nil
	}

	setting, err := sjson.Set("", "index.default_pipeline", pipelineName)
	if err != nil {
		return err
	}

	resp, err := r.osClient.Indices.UpdateIndicesSettings(
		r.ctx,
		[]string{index},
		strings.NewReader(setting),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to update index settings: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) UpdateIndexMappingsForNeuralSearch(index string) error {
	var absent bool
	var err error

	if strings.HasSuffix(index, "*") {
		absent, err = r.prefixIndicesAbsent(index)
		if err != nil {
			return err
		}
	} else {
		exists, err := r.indexExists(index)
		absent = !exists
		if err != nil {
			return err
		}
	}

	// If indices don't exist we don't need to update anything
	if absent {
		return nil
	}

	mapping, err := sjson.Set("", "properties."+types.LogEmbeddingName, types.LogEmbeddingMappings)
	if err != nil {
		return err
	}

	resp, err := r.osClient.Indices.UpdateIndicesMappings(
		r.ctx,
		[]string{index},
		strings.NewReader(mapping),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to update index settings: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) shouldUpdateIngestPipelineForNeuralSearch(pipelineName string, modelID string) (bool, error) {
	if modelID == "" {
		return false, fmt.Errorf("neural search model not loaded into memory")
	}
	resp, err := r.osClient.Ingest.GetIngestPipeline(r.ctx, pipelineName)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return false, fmt.Errorf("failed to get pipeline %s: %s", pipelineName, resp.String())
	}

	existing := types.IngestPipeline{}
	err = json.NewDecoder(resp.Body).Decode(&existing)
	if err != nil {
		return false, err
	}

	for _, p := range existing.Processors {
		if p.TextEmbedding != nil && p.TextEmbedding.ModelId == modelID {
			return false, nil
		}
	}

	return true, nil
}

func (r *Reconciler) CreateNeuralSearchModel(customUrl string) (string, error) {
	resp, err := r.osClient.NeuralSearch.PostEnableModelAccessControl(r.ctx)
	if err != nil {
		return "", fmt.Errorf("failed to enable model access control: %s", resp.String())
	}
	modelGroupID, err := r.osClient.NeuralSearch.MaybeCreateModelGroup(r.ctx)
	if err != nil {
		return "", err
	}
	modelID, err := r.osClient.NeuralSearch.MaybeCreateRegisteredModel(r.ctx, modelGroupID, customUrl)
	if err != nil {
		return "", err
	}
	err = r.osClient.NeuralSearch.DeployNeuralSearchModel(r.ctx, modelID)
	if err != nil {
		return "", err
	}

	return modelID, nil
}

func (r *Reconciler) MaybeUpdateIngestPipelineForNeuralSearch(pipelineName string, pipeline types.IngestPipeline, modelID string) error {
	shouldUpdate, err := r.shouldUpdateIngestPipelineForNeuralSearch(pipelineName, modelID)
	if err != nil {
		return err
	}
	if !shouldUpdate {
		return nil
	}

	pipeline.Processors = append(pipeline.Processors, types.Processor{
		TextEmbedding: &types.TextEmbeddingConfig{
			ModelId: modelID,
			FieldMap: &types.FieldMap{
				Log: types.LogEmbeddingName,
			},
		},
	})

	resp, err := r.osClient.Ingest.PutIngestTemplate(r.ctx, pipelineName, opensearchutil.NewJSONReader(pipeline))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to update pipeline processor: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) DeleteNeuralSearchModel() error {
	resp, err := r.osClient.NeuralSearch.PostSearchExistingModel(r.ctx)
	if err != nil {
		return err
	}

	modelResp := types.ModelGroupSearchResp{}
	err = json.NewDecoder(resp.Body).Decode(&modelResp)
	if err != nil {
		return err
	}
	modelUploaded := len(modelResp.ModelGroupHits.Hits) > 0
	if !modelUploaded {
		return nil // model already deleted, do nothing
	}

	modelID := modelResp.ModelGroupHits.Hits[0].Source.ModelID
	_, err = r.osClient.NeuralSearch.PostUnDeployModel(r.ctx, modelID)
	if err != nil {
		return err
	}
	_, err = r.osClient.NeuralSearch.PostDeleteModel(r.ctx, modelID)
	if err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) UpsertClusterMetadata(id, name, index string) error {
	if name == "" {
		return r.DeleteClusterMetadata(id, index)
	}
	mdDoc := types.ClusterMetadataDocUpdate{
		Name: name,
	}

	upsertRequest := types.MetadataUpdate{
		Document:         mdDoc,
		DocumentAsUpsert: lo.ToPtr(true),
	}

	resp, err := r.osClient.Indices.UpdateDocument(r.ctx, index, id, opensearchutil.NewJSONReader(upsertRequest))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to upsert metadata doc: %s", resp.String())
	}

	return nil
}

func (r *Reconciler) DeleteClusterMetadata(id, index string) error {
	resp, err := r.osClient.Indices.DeleteByID(r.ctx, index, id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.IsError() {
		return fmt.Errorf("failed to delete metadata doc: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) shouldUpdateRepository(name string, settings types.RepositoryRequest) (bool, error) {
	lg := log.FromContext(r.ctx)

	resp, err := r.osClient.Snapshot.GetRepository(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return true, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	existing := types.RepositoryRequest{}
	err = json.NewDecoder(resp.Body).Decode(&existing)
	if err != nil {
		return false, err
	}

	if !reflect.DeepEqual(settings, existing) {
		lg.Info("repository settings exist but are different")
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) repositoryExists(name string) (bool, error) {
	resp, err := r.osClient.Snapshot.GetRepository(r.ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return true, nil
}

func (r *Reconciler) MaybeUpdateRepository(name string, settings types.RepositoryRequest) error {
	update, err := r.shouldUpdateRepository(name, settings)
	if err != nil {
		return fmt.Errorf("failed to check if repository should be updated: %w", err)
	}

	if !update {
		return nil
	}

	resp, err := r.osClient.Snapshot.PutRepository(r.ctx, name, opensearchutil.NewJSONReader(settings))
	if err != nil {
		return fmt.Errorf("failed to register repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to register repository: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) MaybeDeleteRepository(name string) error {
	exists, err := r.repositoryExists(name)
	if err != nil {
		return fmt.Errorf("failed to check if repository exists: %w", err)
	}

	if !exists {
		return nil
	}

	resp, err := r.osClient.Snapshot.DeleteRepository(r.ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to delete repository: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) snapshotExists(name, repository string) (bool, error) {
	resp, err := r.osClient.Snapshot.GetSnapshot(r.ctx, name, repository)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return true, nil
}

func (r *Reconciler) CreateSnapshotAsync(name, repository string, settings types.SnapshotRequest) error {
	exists, err := r.snapshotExists(name, repository)
	if err != nil {
		return err
	}
	if exists {
		return oserrors.ErrSnapshotAlreadyExsts
	}

	resp, err := r.osClient.Snapshot.CreateSnapshot(r.ctx, name, repository, opensearchutil.NewJSONReader(settings), false)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("failed to create snapshot: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) GetSnapshotState(name, repository string) (types.SnapshotState, string, error) {
	resp, err := r.osClient.Snapshot.GetSnapshot(r.ctx, name, repository)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", "", fmt.Errorf("snapshot %s missing in repo %s: %w", name, repository, oserrors.ErrNotFound)
	} else if resp.IsError() {
		return "", "", fmt.Errorf("response from API is %s", resp.String())
	}

	snapshotResp := types.SnapshotResponse{}
	err = json.NewDecoder(resp.Body).Decode(&snapshotResp)
	if err != nil {
		return "", "", err
	}

	snapshot := snapshotResp.Snapshots[0]

	switch snapshot.State {
	case types.SnapshotStateFailed, types.SnapshotStatePartial:
		return snapshot.State, strings.Join(snapshot.Failures, "\n"), nil
	default:
		return snapshot.State, "", nil
	}

}

func (r *Reconciler) GetSnapshotPolicy(name string) (types.SnapshotManagementResponse, error) {
	policy := types.SnapshotManagementResponse{}

	resp, err := r.osClient.Snapshot.GetSnapshotPolicy(r.ctx, name)
	if err != nil {
		return policy, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return policy, fmt.Errorf("could not find policy %s: %w", name, oserrors.ErrNotFound)
	} else if resp.IsError() {
		return policy, fmt.Errorf("response from API is %s", resp.String())
	}

	err = json.NewDecoder(resp.Body).Decode(&policy)
	return policy, err
}

func (r *Reconciler) MaybeUpdateSnapshotPolicy(name string, policy types.SnapshotManagementRequest) error {
	lg := log.FromContext(r.ctx)
	oldPolicy, err := r.GetSnapshotPolicy(name)
	if err != nil {
		if !errors.Is(err, oserrors.ErrNotFound) {
			return err
		}
		lg.Info("snapshot policy does not exist, creating")
		resp, err := r.osClient.Snapshot.CreateSnapshotPolicy(r.ctx, name, opensearchutil.NewJSONReader(policy))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return fmt.Errorf("response from API is %s", resp.String())
		}
		return nil
	}

	if !r.snapshotPolicyNeedsUpdate(policy, oldPolicy.Policy) {
		return nil
	}

	lg.Info("snapshot policy is different, updating")

	resp, err := r.osClient.Snapshot.UpdateSnapshotPolicy(
		r.ctx,
		name,
		oldPolicy.SeqNo,
		oldPolicy.PrimaryTerm,
		opensearchutil.NewJSONReader(policy),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("response from API is %s", resp.String())
	}
	return nil
}

func (r *Reconciler) MaybeDeleteSnapshotPolicy(name string) error {
	_, err := r.GetSnapshotPolicy(name)
	if err != nil {
		if !errors.Is(err, oserrors.ErrNotFound) {
			return fmt.Errorf("failed to check policy: %w", err)
		}
		return nil
	}

	resp, err := r.osClient.Snapshot.DeleteSnapshotPolicy(r.ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete policy: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to delete policy: %s", resp.String())
	}
	return nil
}

func (r *Reconciler) NextSnapshotPolicyTrigger(name string) (time.Duration, error) {
	resp, err := r.osClient.Snapshot.ExplainSnapshotPolicy(r.ctx, name)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return 0, fmt.Errorf("response from API is %s", resp.String())
	}

	explain := types.SnapshotPolicyExplain{}
	err = json.NewDecoder(resp.Body).Decode(&explain)
	if err != nil {
		return 0, err
	}

	if len(explain.Policies) != 1 {
		return 0, errors.New("did not get exactly 1 policy")
	}

	duration := time.Until(explain.Policies[0].Creation.Trigger.Time.Time)
	if duration <= 0 {
		duration = time.Minute
	}

	return duration, nil
}

func (r *Reconciler) GetSnapshotPolicyLastExecution(name string) (*types.SnapshotPolicyExecution, error) {
	resp, err := r.osClient.Snapshot.ExplainSnapshotPolicy(r.ctx, name)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return nil, fmt.Errorf("response from API is %s", resp.String())
	}

	explain := types.SnapshotPolicyExplain{}
	err = json.NewDecoder(resp.Body).Decode(&explain)
	if err != nil {
		return nil, err
	}

	if len(explain.Policies) != 1 {
		return nil, errors.New("did not get exactly 1 policy")
	}

	return &explain.Policies[0].Creation.LatestExecution, nil
}

func (r *Reconciler) snapshotPolicyNeedsUpdate(new, current types.SnapshotManagementRequest) bool {
	lg := log.FromContext(r.ctx)
	if new.Description != current.Description {
		lg.Info("description is different", "current", current.Description, "new", new.Description)
		return true
	}

	if lo.FromPtrOr(new.Enabled, true) != lo.FromPtrOr(current.Enabled, true) {
		lg.Info("enabled is different", "current", lo.FromPtrOr(current.Enabled, true), "new", lo.FromPtrOr(new.Enabled, true))
		return true
	}

	if !reflect.DeepEqual(new.SnapshotConfig, current.SnapshotConfig) {
		lg.Info(fmt.Sprintf("config is different; current %+v new %+v", current.Description, new.Description))
		return true
	}

	if !reflect.DeepEqual(new.Creation, current.Creation) {
		lg.Info(fmt.Sprintf("creation is different; current %+v new %+v", current.Creation, new.Creation))
		return true
	}

	// Comparing the deletion policy needs some extra logic due to default values
	if new.Deletion != nil {
		if current.Deletion == nil {
			return true
		}

		if new.Deletion.Schedule == nil {
			if !reflect.DeepEqual(new.Creation.Schedule, current.Deletion.Schedule) {
				lg.Info(fmt.Sprintf("deletion schedule is different; current %+v new %+v", current.Deletion.Schedule, new.Creation.Schedule))
				return true
			}
		}

		if lo.FromPtrOr(new.Deletion.Condition.MinCount, 1) != lo.FromPtrOr(current.Deletion.Condition.MinCount, 1) {
			lg.Info(fmt.Sprintf(
				"deletion min count is different; current %d new %d",
				lo.FromPtrOr(current.Deletion.Condition.MinCount, 1),
				lo.FromPtrOr(new.Deletion.Condition.MinCount, 1),
			))
			return true
		}

		if lo.FromPtr(new.Deletion.Condition.MaxCount) != lo.FromPtr(current.Deletion.Condition.MaxCount) {
			lg.Info(fmt.Sprintf(
				"deletion max count is different; current %d new %d",
				lo.FromPtr(current.Deletion.Condition.MaxCount),
				lo.FromPtr(new.Deletion.Condition.MaxCount),
			))
			return true
		}

		if new.Deletion.Condition.MaxAge != current.Deletion.Condition.MaxAge {
			lg.Info(fmt.Sprintf("deletion max ageis different; current %s new %s", current.Deletion.Condition.MaxAge, new.Deletion.Condition.MaxAge))
			return true
		}

		if new.Deletion.TimeLimit != current.Deletion.TimeLimit {
			lg.Info(fmt.Sprintf("deletion time limit is different; current %s new %s", current.Deletion.TimeLimit, new.Deletion.TimeLimit))
			return true
		}

		return false
	}

	lg.Info("new deletion is empty, but current isn't")
	if !reflect.DeepEqual(new.Creation.Schedule, current.Deletion.Schedule) {
		lg.Info(fmt.Sprintf("deletion schedule is different; current %+v new %+v", current.Deletion.Schedule, new.Creation.Schedule))
		return true
	}

	return false
}
