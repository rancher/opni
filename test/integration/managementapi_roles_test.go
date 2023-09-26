package integration_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/test"
)

//#region Test Setup

var _ = Describe("Management API Roles Management Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var capability *corev1.CapabilityType
	BeforeAll(func() {
		capability = &corev1.CapabilityType{
			Name: wellknown.CapabilityExample,
		}
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)
		client = environment.NewManagementClient()
	})

	//#endregion

	//#region Happy Path Tests

	var err error
	When("creating a new role", func() {

		It("can get information about all roles", func() {
			_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
				Capability: capability,
				Role: &corev1.Role{
					Id: "test-role1",
					Permissions: []*corev1.PermissionItem{
						{
							Type: string(corev1.PermissionTypeCluster),
							Ids:  []string{"test-cluster"},
							MatchLabels: &corev1.LabelSelector{
								MatchLabels: map[string]string{"test-label": "test-value"},
							},
							Verbs: []*corev1.PermissionVerb{
								{Verb: "GET"},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			roleInfo, err := client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
				Capability: capability,
				RoleRef: &corev1.Reference{
					Id: "test-role1",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(roleInfo.Id).To(Equal("test-role1"))
			Expect(roleInfo.Permissions[0].Ids).To(Equal([]string{"test-cluster"}))
			Expect(roleInfo.Permissions[0].MatchLabels.GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))
		})
	})

	It("can list all roles", func() {
		role, err := client.ListBackendRoles(context.Background(), capability)
		Expect(err).NotTo(HaveOccurred())

		roleList := role.GetItems().GetItems()
		Expect(roleList).To(HaveLen(1))
		for _, roleItem := range roleList {
			Expect(roleItem.Id).To(Equal("test-role1"))
		}
	})

	It("can update an existing role", func() {
		_, err := client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role1",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		role := &corev1.Role{
			Id: "test-role1",
			Permissions: []*corev1.PermissionItem{
				{
					Type: string(corev1.PermissionTypeCluster),
					Ids:  []string{"updated-test-cluster"},
					MatchLabels: &corev1.LabelSelector{
						MatchLabels: map[string]string{"test-label": "updated-test-value"},
					},
					Verbs: []*corev1.PermissionVerb{
						{Verb: "GET"},
					},
				},
			},
		}
		_, err = client.UpdateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role:       role,
		})
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role1",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(roleInfo.Id).To(Equal("test-role1"))
		Expect(roleInfo.Permissions[0].Ids).To(Equal([]string{"updated-test-cluster"}))
		Expect(roleInfo.Permissions[0].MatchLabels.GetMatchLabels()).To(Equal(map[string]string{"test-label": "updated-test-value"}))
	})

	It("can delete an existing role", func() {
		_, err := client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role1",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role1",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot create a role without an Id", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("InvalidArgument desc = missing required field: id"))
	})

	It("can create and get a role without a cluster ID", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role2",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role2",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(roleInfo.Id).To(Equal("test-role2"))
		Expect(roleInfo.Permissions[0].Ids).To(BeNil())
		Expect(roleInfo.Permissions[0].GetMatchLabels().GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role2",
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can create and get a role without a label", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role3",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.GetBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role3",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(roleInfo.Id).To(Equal("test-role3"))
		Expect(roleInfo.Permissions[0].Ids).To(Equal([]string{"test-cluster"}))
		Expect(roleInfo.Permissions[0].MatchLabels).To(BeNil())

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role3",
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete an existing role without specifying an Id", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role4",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef:    &corev1.Reference{},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role4",
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete an existing role without specifying a valid Id", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role5",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: uuid.NewString(),
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role5",
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot create roles with identical Ids", func() {
		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role6",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "test-role6",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						MatchLabels: &corev1.LabelSelector{
							MatchLabels: map[string]string{"test-label": "test-value"},
						},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.AlreadyExists))

		_, err = client.DeleteBackendRole(context.Background(), &corev1.BackendRoleRequest{
			Capability: capability,
			RoleRef: &corev1.Reference{
				Id: "test-role6",
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot update a non existent role", func() {
		_, err = client.UpdateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "does-not-exist",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Ids:  []string{"test-cluster"},
						Verbs: []*corev1.PermissionVerb{
							{Verb: "GET"},
						},
					},
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	//#endregion

})
