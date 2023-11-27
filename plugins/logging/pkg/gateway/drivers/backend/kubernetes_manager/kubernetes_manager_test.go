package kubernetes_manager_test

import (
	"context"
	"encoding/json"
	"time"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test/testlog"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend/kubernetes_manager"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Opensearch Backend", Ordered, Label("integration"), func() {
	var (
		namespace string
		manager   *kubernetes_manager.KubernetesManagerDriver

		timeout  = 30 * time.Second
		interval = time.Second
	)

	BeforeEach(func() {
		namespace = "test-roles"
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(func() error {
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ns), &corev1.Namespace{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return k8sClient.Create(context.Background(), ns)
				}
				return err
			}
			return nil
		}()).To(Succeed())
		opensearch := &opsterv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni",
				Namespace: namespace,
			},
			Spec: opsterv1.ClusterSpec{
				NodePools: []opsterv1.NodePool{
					{
						Component: "main",
						Roles: []string{
							"data",
							"controlplane",
						},
						Replicas: 3,
					},
				},
			},
		}
		Expect(func() error {
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(opensearch), &opsterv1.OpenSearchCluster{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return k8sClient.Create(context.Background(), opensearch)
				}
				return err
			}
			return nil
		}()).To(Succeed())
	})

	JustBeforeEach(func() {
		opniCluster := &opnimeta.OpensearchClusterRef{
			Name:      "opni",
			Namespace: namespace,
		}
		var err error
		manager, err = kubernetes_manager.NewKubernetesManagerDriver(
			kubernetes_manager.KubernetesManagerDriverOptions{
				K8sClient:         k8sClient,
				OpensearchCluster: opniCluster,
				Namespace:         namespace,
				Logger:            testlog.Log,
			},
		)
		Expect(err).NotTo(HaveOccurred())
	})
	When("role does not exist", Ordered, func() {
		Specify("delete should error", func() {
			Expect(manager.DeleteRole(context.Background(), &v1.Reference{
				Id: "test",
			})).NotTo(Succeed())
		})
		Specify("update should error", func() {
			Expect(manager.UpdateRole(context.Background(), &v1.Role{
				Id: "test",
				Permissions: []*v1.PermissionItem{
					{},
				},
			})).NotTo(Succeed())
		})
		Specify("create should succeed and create the role", func() {
			By("checking create succeeds and creates the k8s object")
			err := manager.CreateRole(context.Background(), &v1.Role{
				Id: "test",
				Permissions: []*v1.PermissionItem{
					{
						Type: string(v1.PermissionTypeCluster),
						Verbs: []*v1.PermissionVerb{
							v1.VerbGet(),
						},
						Ids: []string{"test"},
						MatchLabels: &v1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
							},
						},
					},
					{
						Type: string(v1.PermissionTypeNamespace),
						Verbs: []*v1.PermissionVerb{
							v1.VerbGet(),
						},
						Ids: []string{"test"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			role := &opsterv1.OpensearchRole{}
			Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "test",
				Namespace: namespace,
			}, role), timeout, interval).Should(Succeed())

			By("checking the appropriate metadata is set")
			_, ok := role.Annotations["opni.io/label_matcher"]
			Expect(ok).To(BeTrue())
			_, ok = role.Labels["opni.io/managed"]
			Expect(ok).To(BeTrue())

			By("checking the role content")
			Expect(len(role.Spec.IndexPermissions)).To(Equal(1))
			perm := role.Spec.IndexPermissions[0]
			Expect(perm.IndexPatterns).To(ContainElement("logs*"))
			query := &kubernetes_manager.DocumentQuery{}
			err = json.Unmarshal([]byte(perm.DocumentLevelSecurity), query)
			Expect(err).NotTo(HaveOccurred())
			Expect(query.Bool.Must).To(ConsistOf(
				kubernetes_manager.Query{
					Term: map[string]kubernetes_manager.TermValue{
						"cluster_id": {
							Value: "test",
						},
					},
				},
				kubernetes_manager.Query{
					Term: map[string]kubernetes_manager.TermValue{
						"namespace": {
							Value: "test",
						},
					},
				},
			))
		})
	})
	When("the role does exist", Ordered, func() {
		Specify("create should error", func() {
			err := manager.CreateRole(context.Background(), &v1.Role{
				Id: "test",
				Permissions: []*v1.PermissionItem{
					{
						Type: string(v1.PermissionTypeCluster),
						Verbs: []*v1.PermissionVerb{
							v1.VerbGet(),
						},
						Ids: []string{"test"},
						MatchLabels: &v1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
							},
						},
					},
					{
						Type: string(v1.PermissionTypeNamespace),
						Verbs: []*v1.PermissionVerb{
							v1.VerbGet(),
						},
						Ids: []string{"test"},
					},
				},
			})
			Expect(err).To(HaveOccurred())
		})
		When("resource versions match", func() {
			Specify("update should succeed", func() {
				role := &opsterv1.OpensearchRole{}
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "test",
					Namespace: namespace,
				}, role)).To(Succeed())
				err := manager.UpdateRole(context.Background(), &v1.Role{
					Id: "test",
					Permissions: []*v1.PermissionItem{
						{
							Type: string(v1.PermissionTypeCluster),
							Verbs: []*v1.PermissionVerb{
								v1.VerbGet(),
							},
							Ids: []string{
								"foo",
								"bar",
							},
						},
					},
					Metadata: &v1.RoleMetadata{
						ResourceVersion: role.ResourceVersion,
					},
				})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "test",
						Namespace: namespace,
					}, role)
					if err != nil {
						return true
					}
					_, ok := role.Annotations["opni.io/label_matcher"]
					return ok
				}(), interval, timeout).Should(BeFalse())

				By("checking the role content")
				Expect(len(role.Spec.IndexPermissions)).To(Equal(1))
				perm := role.Spec.IndexPermissions[0]
				Expect(perm.IndexPatterns).To(ContainElement("logs*"))
				query := &kubernetes_manager.DocumentQuery{}
				err = json.Unmarshal([]byte(perm.DocumentLevelSecurity), query)
				Expect(err).NotTo(HaveOccurred())
				Expect(query.Bool.Must).To(ConsistOf(
					kubernetes_manager.Query{
						Terms: map[string][]string{
							"cluster_id": {
								"foo",
								"bar",
							},
						},
					},
				))
			})
		})
		When("resource versions don't match", func() {
			Specify("update should error", func() {
				err := manager.UpdateRole(context.Background(), &v1.Role{
					Id: "test",
					Permissions: []*v1.PermissionItem{
						{
							Type: string(v1.PermissionTypeCluster),
							Verbs: []*v1.PermissionVerb{
								v1.VerbGet(),
							},
							Ids: []string{
								"foo",
								"bar",
							},
						},
					},
				})
				Expect(err).To(HaveOccurred())
			})
		})
		Specify("list should return the role", func() {
			list, err := manager.ListRoles(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list.GetItems()).To(ConsistOf(
				&v1.Reference{
					Id: "test",
				},
			))
		})
		Specify("delete should succeed", func() {
			Expect(manager.DeleteRole(context.Background(), &v1.Reference{
				Id: "test",
			})).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "test",
					Namespace: namespace,
				}, &opsterv1.OpensearchRole{})
				if err == nil {
					return false
				}
				return k8serrors.IsNotFound(err)
			}(), interval, timeout).Should(BeTrue())
		})
	})

	When("an unmanaged role exists", Ordered, func() {
		BeforeAll(func() {
			role := &opsterv1.OpensearchRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: namespace,
				},
				Spec: opsterv1.OpensearchRoleSpec{
					OpensearchRef: corev1.LocalObjectReference{
						Name: "opni",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), role)).To(Succeed())
		})
		Specify("list should return no results", func() {
			list, err := manager.ListRoles(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(len(list.GetItems())).To(Equal(0))
		})
		Specify("update should error", func() {
			err := manager.UpdateRole(context.Background(), &v1.Role{
				Id: "test",
				Permissions: []*v1.PermissionItem{
					{
						Type: string(v1.PermissionTypeCluster),
						Verbs: []*v1.PermissionVerb{
							v1.VerbGet(),
						},
						Ids: []string{
							"foo",
							"bar",
						},
					},
				},
			})
			Expect(err).To(HaveOccurred())
		})
	})
	Specify("delete should error", func() {
		Expect(manager.DeleteRole(context.Background(), &v1.Reference{
			Id: "test",
		})).NotTo(Succeed())
	})
})
