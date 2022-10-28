package integration_test

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
)

//#region Test Setup

var _ = Describe("Management API Roles Management Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	var err error
	When("creating a new role", func() {

		It("can get information about all roles", func() {
			_, err = client.CreateRole(context.Background(), &corev1.Role{
				Id:         "test-role",
				ClusterIDs: []string{"test-cluster"},
				MatchLabels: &corev1.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			},
			)
			Expect(err).NotTo(HaveOccurred())

			roleInfo, err := client.GetRole(context.Background(), &corev1.Reference{
				Id: "test-role",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(roleInfo.Id).To(Equal("test-role"))
			Expect(roleInfo.ClusterIDs).To(Equal([]string{"test-cluster"}))
			Expect(roleInfo.MatchLabels.GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))
		})
	})

	It("can list all roles", func() {
		role, err := client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		roleList := role.Items
		Expect(roleList).To(HaveLen(1))
		for _, roleItem := range roleList {
			Expect(roleItem.Id).To(Equal("test-role"))
			Expect(roleItem.ClusterIDs).To(Equal([]string{"test-cluster"}))
			Expect(roleItem.MatchLabels.GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))
		}
	})

	It("can delete an existing role", func() {
		_, err := client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot create a role without an Id", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			ClusterIDs: []string{"test-cluster"},
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Unknown desc = missing required field: id"))

		_, err = client.GetRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("can create and get a role without a cluster ID", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id: "test-role",
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.GetRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(roleInfo.Id).To(Equal("test-role"))
		Expect(roleInfo.ClusterIDs).To(BeNil())
		Expect(roleInfo.GetMatchLabels().GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can create and get a role without a label", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster"},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.GetRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(roleInfo.Id).To(Equal("test-role"))
		Expect(roleInfo.ClusterIDs).To(Equal([]string{"test-cluster"}))
		Expect(roleInfo.MatchLabels).To(BeNil())

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete an existing role without specifying an Id", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster"},
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete an existing role without specifying a valid Id", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster"},
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	//TODO: This can be unignored once this functionality is implemented
	XIt("cannot create roles with identical Ids", func() {
		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster"},
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster"},
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			},
		},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create role: already exists"))

		_, err = client.DeleteRole(context.Background(), &corev1.Reference{
			Id: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	//#endregion

})
