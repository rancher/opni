package reconciler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/rancher/opni/pkg/opensearch/dashboards"
	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/opensearch/opensearch/errors"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/samber/lo"
)

var _ = Describe("Opensearch", Ordered, Label("unit"), func() {
	var (
		rec       *reconciler.Reconciler
		transport *httpmock.MockTransport

		logPolicyName               = "log-policy"
		logIndexAlias               = "logs"
		kibanaDashboardVersionDocID = "latest"
		kibanaDashboardVersion      = "v0.1.3"
		kibanaDashboardVersionIndex = "opni-dashboard-version"

		opensearchURL          = "https://mock-opensearch.example.com"
		dashboardsURL          = "https://mock-dashboards.example.com"
		dashboardsUser         = "test"
		dashboardsPassword     = "test"
		dashboardsURLWithCreds = "https://test:test@mock-dashboards.example.com"
	)

	BeforeEach(func() {
		transport = httpmock.NewMockTransport()
		transport.RegisterNoResponder(httpmock.NewNotFoundResponder(nil))
	})

	JustBeforeEach(func() {
		opensearchClient, err := opensearch.NewClient(
			opensearch.ClientConfig{
				URLs: []string{
					opensearchURL,
				},
				Username:   "test-user",
				CertReader: &mockCertReader{},
			},
			opensearch.WithTransport(transport),
		)
		Expect(err).NotTo(HaveOccurred())

		dashboardsClient, err := dashboards.NewClient(
			dashboards.Config{
				URL:      dashboardsURL,
				Username: dashboardsUser,
				Password: dashboardsPassword,
			},
			dashboards.WithTransport(transport),
		)
		Expect(err).NotTo(HaveOccurred())

		rec, err = reconciler.NewReconciler(
			context.Background(),
			reconciler.ReconcilerConfig{},
			reconciler.WithOpensearchClient(opensearchClient),
			reconciler.WithDashboardshClient(dashboardsClient),
		)
		Expect(err).NotTo(HaveOccurred())
	})
	Context("reconciling ISM policies", func() {
		var (
			policy         types.ISMPolicySpec
			policyResponse *types.ISMGetResponse
		)
		BeforeEach(func() {
			policy = types.ISMPolicySpec{
				ISMPolicyIDSpec: &types.ISMPolicyIDSpec{
					PolicyID:   "testpolicy",
					MarshallID: false,
				},
				Description:  "testing policy",
				DefaultState: "test",
				States: []types.StateSpec{
					{
						Name: "test",
						Actions: []types.ActionSpec{
							{
								ActionOperation: &types.ActionOperation{
									ReadOnly: &types.ReadOnlyOperation{},
								},
							},
						},
						Transitions: make([]types.TransitionSpec, 0),
					},
				},
				ISMTemplate: []types.ISMTemplateSpec{
					{
						IndexPatterns: []string{
							"test*",
						},
						Priority: 100,
					},
				},
			}
			policyResponse = &types.ISMGetResponse{
				ID:          "testid",
				Version:     1,
				PrimaryTerm: 1,
				SeqNo:       1,
				Policy: types.ISMPolicySpec{
					ISMPolicyIDSpec: &types.ISMPolicyIDSpec{
						PolicyID:   "testpolicy",
						MarshallID: true,
					},
					Description:  "testing policy",
					DefaultState: "test",
					States: []types.StateSpec{
						{
							Name: "test",
							Actions: []types.ActionSpec{
								{
									ActionOperation: &types.ActionOperation{
										ReadOnly: &types.ReadOnlyOperation{},
									},
								},
							},
							Transitions: make([]types.TransitionSpec, 0),
						},
					},
					ISMTemplate: []types.ISMTemplateSpec{
						{
							IndexPatterns: []string{
								"test*",
							},
							Priority: 100,
						},
					},
				},
			}
		})
		When("ISM does not exist", func() {
			It("should create a new ISM", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_ism/policies/testpolicy", opensearchURL),
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_plugins/_ism/policies/testpolicy", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, policyResponse).Once(),
				)
				Expect(func() error {
					err := rec.ReconcileISM(policy)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("ISM exists and is the same", func() {
			It("should do nothing", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_ism/policies/testpolicy", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, policyResponse).Once(),
				)
				Expect(func() error {
					err := rec.ReconcileISM(policy)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("ISM exists and is different", func() {
			It("should update the policy", func() {
				policyResponseNew := &types.ISMGetResponse{
					ID:          "testid",
					Version:     1,
					PrimaryTerm: 1,
					SeqNo:       2,
					Policy: types.ISMPolicySpec{
						ISMPolicyIDSpec: &types.ISMPolicyIDSpec{
							PolicyID:   "testpolicy",
							MarshallID: true,
						},
						Description:  "testing policy",
						DefaultState: "test",
						States: []types.StateSpec{
							{
								Name: "test",
								Actions: []types.ActionSpec{
									{
										ActionOperation: &types.ActionOperation{
											ReadOnly: &types.ReadOnlyOperation{},
										},
									},
								},
								Transitions: make([]types.TransitionSpec, 0),
							},
						},
						ISMTemplate: []types.ISMTemplateSpec{
							{
								IndexPatterns: []string{
									"test*",
								},
								Priority: 100,
							},
						},
					},
				}
				policy.Description = "this is a different description"
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_ism/policies/testpolicy", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, policyResponse).Once(),
				)
				transport.RegisterResponderWithQuery(
					"PUT",
					fmt.Sprintf("%s/_plugins/_ism/policies/testpolicy", opensearchURL),
					map[string]string{
						"if_seq_no":       "1",
						"if_primary_term": "1",
					},
					httpmock.NewJsonResponderOrPanic(200, policyResponseNew).Once(),
				)
				Expect(func() error {
					err := rec.ReconcileISM(policy)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling index templates", func() {
		var indexTemplate types.IndexTemplateSpec
		BeforeEach(func() {
			indexTemplate = types.IndexTemplateSpec{
				TemplateName: "testtemplate",
				IndexPatterns: []string{
					"test*",
				},
				Template: types.TemplateSpec{
					Settings: types.TemplateSettingsSpec{
						NumberOfShards:   1,
						NumberOfReplicas: 1,
						ISMPolicyID:      logPolicyName,
						RolloverAlias:    logIndexAlias,
					},
					Mappings: types.TemplateMappingsSpec{
						Properties: map[string]types.PropertySettings{
							"timestamp": {
								Type: "date",
							},
						},
					},
				},
			}
		})
		When("the template does not exist", func() {
			It("should create the index template", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_index_template/testtemplate", opensearchURL),
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_index_template/testtemplate", opensearchURL),
					httpmock.NewStringResponder(200, `{"status": "complete"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateIndexTemplate(indexTemplate)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("the template does exist", func() {
			It("should do nothing", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_index_template/testtemplate", opensearchURL),
					httpmock.NewStringResponder(200, `{"mesg": "found it"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateIndexTemplate(indexTemplate)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling rollover indices", func() {
		var (
			prefix = "test"
			alias  = "test"
		)
		When("alias index, and rollover indices don't exist", func() {
			It("should bootstrap the index", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test*", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, "[]").Once(),
				)
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"POST",
					fmt.Sprintf("%s/_aliases", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/test-000001", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeBootstrapIndex(prefix, alias, []string{})
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("alias index exists, and rollover indices don't", func() {
			It("should reindex into the bootstrap index", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test*", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, "[]").Once(),
				)
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, `{"status": "exists"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/test-000001", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponderWithQuery(
					"POST",
					fmt.Sprintf("%s/_reindex", opensearchURL),
					map[string]string{
						"wait_for_completion": "true",
					},
					httpmock.NewStringResponder(200, `{"status": "OK"}`).Once(),
				)
				transport.RegisterResponder(
					"POST",
					fmt.Sprintf("%s/_aliases", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeBootstrapIndex(prefix, alias, []string{})
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("rollover indices exist", func() {
			It("should do nothing", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test*", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, `[{"test-000002": "thisexists"}, {"test-000003": "this also exists"}]`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeBootstrapIndex(prefix, alias, []string{})
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling indices", func() {
		var (
			indexName     string
			indexSettings = map[string]types.TemplateMappingsSpec{}
		)
		BeforeEach(func() {
			indexName = "test"
			indexSettings = map[string]types.TemplateMappingsSpec{
				"mappings": {
					Properties: map[string]types.PropertySettings{
						"start_ts": {
							Type:   "date",
							Format: "epoch_millis",
						},
						"end_ts": {
							Type:   "date",
							Format: "epoch_millis",
						},
					},
				},
			}
		})
		When("index does not exist", func() {
			It("should create the index settings", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/test", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateIndex(indexName, indexSettings)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("index does exist", func() {
			It("should do nothing", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/test", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateIndex(indexName, indexSettings)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling security objects", func() {
		var (
			role  types.RoleSpec
			user  types.UserSpec
			users []string
		)
		BeforeEach(func() {
			role = types.RoleSpec{
				RoleName: "test_role",
				ClusterPermissions: []string{
					"cluster_composite_ops_ro",
				},
				IndexPermissions: []types.IndexPermissionSpec{
					{
						IndexPatterns: []string{
							"logs*",
						},
						AllowedActions: []string{
							"read",
							"search",
						},
					},
				},
			}
			user = types.UserSpec{
				UserName: "test",
				Password: "test",
			}
			// roleMapping = map[string]types.RoleMappingSpec{
			// 	role.RoleName: types.RoleMappingSpec{
			// 		Users: []string{
			// 			user.UserName,
			// 		},
			// 	},
			// }
		})
		When("role does not exist", func() {
			It("should the role", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/roles/test_role", opensearchURL),
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_plugins/_security/api/roles/test_role", opensearchURL),
					httpmock.NewStringResponder(200, `{"status": "created"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateRole(role)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("role exists", func() {
			It("should do nothing", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/roles/test_role", opensearchURL),
					httpmock.NewStringResponder(200, `{"status": "ok"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateRole(role)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("user does not exist", func() {
			It("should create the user", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/internalusers/test", opensearchURL),
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_plugins/_security/api/internalusers/test", opensearchURL),
					httpmock.NewStringResponder(200, `{"status": "created"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateUser(user)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("user exists", func() {
			It("should do nothing", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/internalusers/test", opensearchURL),
					httpmock.NewStringResponder(200, `{"status": "ok"}`).Once(),
				)
				Expect(func() error {
					err := rec.MaybeCreateUser(user)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("role mappiing does not exist", func() {
			It("should create the role mapping", func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/rolesmapping/test_role", opensearchURL),
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_plugins/_security/api/rolesmapping/test_role", opensearchURL),
					func(req *http.Request) (*http.Response, error) {
						mapping := &types.RoleMappingSpec{}
						if err := json.NewDecoder(req.Body).Decode(&mapping); err != nil {
							return httpmock.NewStringResponse(501, ""), nil
						}
						users = mapping.Users
						return httpmock.NewStringResponse(200, ""), nil
					},
				)
				Expect(func() error {
					err := rec.MaybeUpdateRolesMapping(role.RoleName, user.UserName)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				Expect(users).To(ContainElement(user.UserName))
			})
		})
		When("role mapping exists and doesn't contain user", func() {
			It("should update the role mapping", func() {
				users = []string{
					"otheruser",
				}
				roleMappingBody := types.RoleMappingReponse{
					role.RoleName: types.RoleMappingSpec{
						Users: users,
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/rolesmapping/test_role", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, roleMappingBody).Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_plugins/_security/api/rolesmapping/test_role", opensearchURL),
					func(req *http.Request) (*http.Response, error) {
						mapping := &types.RoleMappingSpec{}
						if err := json.NewDecoder(req.Body).Decode(&mapping); err != nil {
							return httpmock.NewStringResponse(501, ""), nil
						}
						users = mapping.Users
						return httpmock.NewStringResponse(200, ""), nil
					},
				)
				Expect(func() error {
					err := rec.MaybeUpdateRolesMapping(role.RoleName, user.UserName)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				Expect(users).To(ContainElement(user.UserName))
				Expect(users).To(ContainElement("otheruser"))
			})
		})
		When("role mapping exists and contains the user", func() {
			It("should do nothing", func() {
				roleMappingBody := types.RoleMappingReponse{
					role.RoleName: types.RoleMappingSpec{
						Users: users,
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_security/api/rolesmapping/test_role", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, roleMappingBody).Once(),
				)
				Expect(func() error {
					err := rec.MaybeUpdateRolesMapping(role.RoleName, user.UserName)
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling kibana objects", func() {
		When("kibana tracking index doesn't exist", func() {
			It("should create the tracking index and the kibana objects", func() {
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/opni-dashboard-version", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(404, `{"mesg": "Not found"}`).Once(),
				)
				transport.RegisterResponderWithQuery(
					"POST",
					fmt.Sprintf("%s/api/saved_objects/_import", dashboardsURLWithCreds),
					map[string]string{
						"overwrite": "true",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponder(
					"POST",
					fmt.Sprintf("%s/opni-dashboard-version/_update/latest", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.ImportKibanaObjects(kibanaDashboardVersionIndex, kibanaDashboardVersionDocID, kibanaDashboardVersion, "")
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("kibana tracking index has version that is old", func() {
			It("should update the tracking index and the kibana objects", func() {
				kibanaResponse := types.DashboardsDocResponse{
					Index:       "opni-dashboard-version",
					ID:          "latest",
					SeqNo:       1,
					PrimaryTerm: 1,
					Found:       opensearchapi.BoolPtr(true),
					Source: types.DashboardsVersionDoc{
						DashboardVersion: "0.0.1",
					},
				}
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/opni-dashboard-version", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/opni-dashboard-version/_doc/latest", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, kibanaResponse).Once(),
				)
				transport.RegisterResponderWithQuery(
					"POST",
					fmt.Sprintf("%s/api/saved_objects/_import", dashboardsURLWithCreds),
					map[string]string{
						"overwrite": "true",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponder(
					"POST",
					fmt.Sprintf("%s/opni-dashboard-version/_update/latest", opensearchURL),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.ImportKibanaObjects(kibanaDashboardVersionIndex, kibanaDashboardVersionDocID, kibanaDashboardVersion, "")
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("kibana tracking index exists and is up to date", func() {
			It("should do nothing", func() {
				kibanaResponse := types.DashboardsDocResponse{
					Index:       "opni-dashboard-version",
					ID:          "latest",
					SeqNo:       1,
					PrimaryTerm: 1,
					Found:       opensearchapi.BoolPtr(true),
					Source: types.DashboardsVersionDoc{
						DashboardVersion: kibanaDashboardVersion,
					},
				}
				transport.RegisterResponderWithQuery(
					"GET",
					fmt.Sprintf("%s/_cat/indices/opni-dashboard-version", opensearchURL),
					map[string]string{
						"format": "json",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/opni-dashboard-version/_doc/latest", opensearchURL),
					httpmock.NewJsonResponderOrPanic(200, kibanaResponse).Once(),
				)
				Expect(func() error {
					err := rec.ImportKibanaObjects(kibanaDashboardVersionIndex, kibanaDashboardVersionDocID, kibanaDashboardVersion, "")
					if err != nil {
						log.Println(err)
					}
					return err
				}()).To(BeNil())
				// Confirm all responders have been called
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("reconciling repository objects", func() {
		var (
			repositoryName string
			settings       = types.RepositoryRequest{
				Type: "s3",
				Settings: types.RepositorySettings{
					S3Settings: &types.S3Settings{
						Bucket: "test-bucket",
						Path:   "backups",
					},
				},
			}
		)
		When("repository doesn't exist", Ordered, func() {
			BeforeEach(func() {
				repositoryName = "not-found"
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewStringResponder(404, "Not found").Once(),
				)
			})
			It("should create a new repository", func() {
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeUpdateRepository(repositoryName, settings)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
			It("should not try to delete the repository", func() {
				Expect(func() error {
					err := rec.MaybeDeleteRepository(repositoryName)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("repository does exist and settings are the same", Ordered, func() {
			BeforeEach(func() {
				repositoryName = "found"
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewJsonResponderOrPanic(200, settings).Once(),
				)
			})
			It("should not create a new repository", func() {
				Expect(func() error {
					err := rec.MaybeUpdateRepository(repositoryName, settings)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
			It("should delete the repository", func() {
				transport.RegisterResponder(
					"DELETE",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeDeleteRepository(repositoryName)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("repository does exist and settings are not the same", Ordered, func() {
			BeforeEach(func() {
				repositoryName = "found"
				settingsOld := types.RepositoryRequest{
					Type: "s3",
					Settings: types.RepositorySettings{
						S3Settings: &types.S3Settings{
							Bucket: "old-bucket",
							Path:   "backups",
						},
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewJsonResponderOrPanic(200, settingsOld).Once(),
				)
			})
			It("should update the repository", func() {
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_snapshot/%s", opensearchURL, repositoryName),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
				Expect(func() error {
					err := rec.MaybeUpdateRepository(repositoryName, settings)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
	Context("working with snapshots", func() {
		var (
			repositoryName = "test-repo"
			snapshotName   = "test"
			indices        = []string{"logs", "test"}
			request        = types.SnapshotRequest{
				Indices: strings.Join(indices, ","),
			}

			snapshotState types.SnapshotState
		)
		When("snapshot does not exist", func() {
			BeforeEach(func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_snapshot/%s/%s", opensearchURL, repositoryName, snapshotName),
					httpmock.NewStringResponder(404, "Not found").Once(),
				)
				transport.RegisterResponder(
					"PUT",
					fmt.Sprintf("%s/_snapshot/%s/%s", opensearchURL, repositoryName, snapshotName),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
			})
			It("should create the snapshot", func() {
				Expect(func() error {
					err := rec.CreateSnapshotAsync(snapshotName, repositoryName, request)
					if err != nil {
						GinkgoWriter.Println(err.Error())
					}
					return err
				}()).To(BeNil())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
			It("should error when getting the snapshot", func() {
				Expect(func() error {
					_, _, err := rec.GetSnapshotState(snapshotName, repositoryName)
					return err
				}()).To(MatchError(errors.ErrNotFound))
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders() - 1))
			})
		})
		When("snapshot does exist", func() {
			JustBeforeEach(func() {
				response := types.SnapshotResponse{
					Snapshots: []types.SnapshotStatus{
						{
							State: snapshotState,
							Failures: []string{
								"This is a generic failure",
							},
						},
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_snapshot/%s/%s", opensearchURL, repositoryName, snapshotName),
					httpmock.NewJsonResponderOrPanic(200, response).Once(),
				)
			})
			It("should error when creating the snapshot", func() {
				Expect(rec.CreateSnapshotAsync(snapshotName, repositoryName, request)).To(MatchError(errors.ErrSnapshotAlreadyExsts))
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})

			When("state is completed", func() {
				BeforeEach(func() {
					snapshotState = types.SnapshotStateSuccess
				})
				It("should return success and no failures", func() {
					state, failures, err := rec.GetSnapshotState(snapshotName, repositoryName)
					Expect(err).To(BeNil())
					Expect(failures).To(BeEmpty())
					Expect(state).To(Equal(types.SnapshotStateSuccess))
					Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				})
			})
			When("state is in progress", func() {
				BeforeEach(func() {
					snapshotState = types.SnapshotStateInProgress
				})
				It("should return success and no failures", func() {
					state, failures, err := rec.GetSnapshotState(snapshotName, repositoryName)
					Expect(err).To(BeNil())
					Expect(failures).To(BeEmpty())
					Expect(state).To(Equal(types.SnapshotStateInProgress))
					Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				})
			})
			When("state is failed", func() {
				BeforeEach(func() {
					snapshotState = types.SnapshotStateFailed
				})
				It("should return failed and include failures", func() {
					state, failures, err := rec.GetSnapshotState(snapshotName, repositoryName)
					Expect(err).To(BeNil())
					Expect(failures).To(ContainSubstring("This is a generic failure"))
					Expect(state).To(Equal(types.SnapshotStateFailed))
					Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				})
			})
			When("state is partial", func() {
				BeforeEach(func() {
					snapshotState = types.SnapshotStatePartial
				})
				It("should return failed and include failures", func() {
					state, failures, err := rec.GetSnapshotState(snapshotName, repositoryName)
					Expect(err).To(BeNil())
					Expect(failures).To(ContainSubstring("This is a generic failure"))
					Expect(state).To(Equal(types.SnapshotStatePartial))
					Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
				})
			})
		})
	})
	Context("working with snapshot policies", func() {
		var (
			policyName    = "test-policy"
			policyRequest types.SnapshotManagementRequest
		)
		BeforeEach(func() {
			policyRequest = types.SnapshotManagementRequest{
				Description: "test policy",
				Enabled:     lo.ToPtr(true),
				SnapshotConfig: types.SnapshotConfig{
					SnapshotRequest: types.SnapshotRequest{
						Indices: "test",
					},
				},
				Creation: types.SnapshotCreation{
					Schedule: &types.SnapshotSchedule{
						CronSchedule: &types.SnapshotCronSchedule{
							Expression: "00 1 * * *",
							Timezone:   "Etc/UTC",
						},
					},
				},
			}
		})
		When("policy does not exist", func() {
			JustBeforeEach(func() {
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					httpmock.NewStringResponder(404, "Not found").Once(),
				)
				transport.RegisterResponder(
					"POST",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					httpmock.NewStringResponder(200, "OK").Once(),
				)
			})
			It("should create the policy", func() {
				Expect(rec.MaybeUpdateSnapshotPolicy(policyName, policyRequest)).To(Succeed())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
			It("should not delete the policy", func() {
				Expect(rec.MaybeDeleteSnapshotPolicy(policyName)).To(Succeed())
			})
		})
		When("policy exists but is different", func() {
			JustBeforeEach(func() {
				response := types.SnapshotManagementResponse{
					Version:     1,
					SeqNo:       1,
					PrimaryTerm: 1,
					Policy: types.SnapshotManagementRequest{
						Description: "test policy",
						Enabled:     lo.ToPtr(true),
						SnapshotConfig: types.SnapshotConfig{
							SnapshotRequest: types.SnapshotRequest{
								Indices: "test",
							},
						},
						Creation: types.SnapshotCreation{
							Schedule: &types.SnapshotSchedule{
								CronSchedule: &types.SnapshotCronSchedule{
									Expression: "00 * * * *",
									Timezone:   "Etc/UTC",
								},
							},
						},
						Deletion: &types.SnapshotDeletion{
							Schedule: &types.SnapshotSchedule{
								CronSchedule: &types.SnapshotCronSchedule{
									Expression: "00 * * * *",
									Timezone:   "Etc/UTC",
								},
							},
						},
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					httpmock.NewJsonResponderOrPanic(200, response).Once(),
				)
				transport.RegisterResponderWithQuery(
					"PUT",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					map[string]string{
						"if_seq_no":       "1",
						"if_primary_term": "1",
					},
					httpmock.NewStringResponder(200, "OK").Once(),
				)
			})
			It("should update the policy", func() {
				Expect(rec.MaybeUpdateSnapshotPolicy(policyName, policyRequest)).To(Succeed())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
		When("policy exists and is the same", func() {
			JustBeforeEach(func() {
				response := types.SnapshotManagementResponse{
					Version:     1,
					SeqNo:       1,
					PrimaryTerm: 1,
					Policy: types.SnapshotManagementRequest{
						Description: "test policy",
						Enabled:     lo.ToPtr(true),
						SnapshotConfig: types.SnapshotConfig{
							SnapshotRequest: types.SnapshotRequest{
								Indices: "test",
							},
						},
						Creation: types.SnapshotCreation{
							Schedule: &types.SnapshotSchedule{
								CronSchedule: &types.SnapshotCronSchedule{
									Expression: "00 1 * * *",
									Timezone:   "Etc/UTC",
								},
							},
						},
						Deletion: &types.SnapshotDeletion{
							Schedule: &types.SnapshotSchedule{
								CronSchedule: &types.SnapshotCronSchedule{
									Expression: "00 1 * * *",
									Timezone:   "Etc/UTC",
								},
							},
						},
					},
				}
				transport.RegisterResponder(
					"GET",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					httpmock.NewJsonResponderOrPanic(200, response).Once(),
				)
				transport.RegisterResponder(
					"DELETE",
					fmt.Sprintf("%s/_plugins/_sm/policies/%s", opensearchURL, policyName),
					httpmock.NewJsonResponderOrPanic(200, response).Once(),
				)
			})
			It("should not update the policy", func() {
				Expect(rec.MaybeUpdateSnapshotPolicy(policyName, policyRequest)).To(Succeed())
			})
			It("should delete the policy", func() {
				Expect(rec.MaybeDeleteSnapshotPolicy(policyName)).To(Succeed())
				Expect(transport.GetTotalCallCount()).To(Equal(transport.NumResponders()))
			})
		})
	})
})
