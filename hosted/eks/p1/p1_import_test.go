package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {
	var k8sVersion string
	BeforeEach(func() {
		// assigning cluster nil value so that every new test has a fresh value of the variable
		// this is to avoid using residual value of a cluster in a test that does not use it
		cluster = nil

		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
				err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
			err := helper.DeleteEKSClusterOnAWS(region, clusterName)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	Context("Upgrade Testing", func() {
		var upgradeToVersion string

		BeforeEach(func() {
			if helpers.SkipUpgradeTests {
				Skip(helpers.SkipUpgradeTestsLog)
			}

			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))
			upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
		})

		When("a cluster is imported", func() {

			BeforeEach(func() {
				err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
				Expect(err).To(BeNil())
				cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})

			It("Upgrade version of node group only", func() {
				testCaseID = 88
				upgradeNodeKubernetesVersionGTCPCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			// eks-operator/issues/752
			It("should successfully update a cluster while it is still in updating state", func() {
				testCaseID = 104
				updateClusterInUpdatingState(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			It("Update k8s version of cluster and add node groups", func() {
				testCaseID = 90
				upgradeCPAndAddNgCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})
		})
	})

	It("should successfully Import cluster with ONLY control plane", func() {
		testCaseID = 94
		err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--without-nodegroup")
		Expect(err).To(BeNil())
		cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
		Expect(err).To(BeNil())
		Eventually(func() bool {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return cluster.Transitioning == "error" && cluster.TransitioningMessage == "Cluster must have at least one managed nodegroup or one self-managed node."
		}, "5m", "2s").Should(BeTrue())
		cluster.EKSConfig = cluster.EKSStatus.UpstreamSpec
		cluster, err = helper.AddNodeGroup(cluster, 1, ctx.RancherAdminClient, false, false)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	It("successfully import EKS cluster with self-managed nodes", func() {
		testCaseID = 107
		err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--managed=false")
		Expect(err).To(BeNil())
		cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
	})

	When("a cluster with multiple nodegroups is imported", func() {
		BeforeEach(func() {
			err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			for i := 0; i < 2; i++ {
				err = helper.AddNodeGroupOnAWS(namegen.AppendRandomString("ng"), clusterName, region)
				Expect(err).To(BeNil())
			}
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully Import cluster with at least 2 nodegroups", func() {
			testCaseID = 105
			helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		})
	})

	When("a cluster is imported", func() {

		var _ = BeforeEach(func() {
			err := helper.CreateEKSClusterOnAWS(region, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Delete & re-import cluster", func() {
			testCaseID = 106

			var err error
			err = helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			Eventually(func() string {
				cluster, _ = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				return cluster.ID
			}, "30s", "3s").Should(BeEmpty())

			cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Update cluster logging types", func() {
			testCaseID = 77
			updateLoggingCheck(cluster, ctx.RancherAdminClient)
		})

		It("Update Tags and Labels", func() {
			testCaseID = 81
			updateTagsAndLabels(cluster, ctx.RancherAdminClient)
		})

		It("Add a nodegroup in EKS -> Syncs to Rancher -> Update cluster, the nodegroup is intact", func() {
			testCaseID = 87
			nodepoolcount := len(*cluster.EKSStatus.UpstreamSpec.NodeGroups)
			err := helper.AddNodeGroupOnAWS(namegen.AppendRandomString("ng"), clusterName, region)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return len(*cluster.EKSStatus.UpstreamSpec.NodeGroups) == nodepoolcount+1
			}, "10m", "7s").Should(BeTrue(), "Timed out while waiting for rancher to sync")
			cluster, err = helper.UpdateLogging(cluster, ctx.RancherAdminClient, []string{"authenticator"}, true)
			Expect(err).To(BeNil())

			// verify that the nodegroups are intact
			Expect(*cluster.EKSStatus.UpstreamSpec.NodeGroups).To(HaveLen(nodepoolcount + 1))
		})

		It("Update the cloud creds", func() {
			testCaseID = 155
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})

		Context("Reimporting/Editing a cluster with invalid config", func() {
			It("Reimport a cluster to Rancher should fail", func() {
				testCaseID = 101

				// We do not assign the cluster returned by import function to `cluster` since it will be nil and the cluster won't be deleted in AfterEach
				_, err := helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("cluster already exists for EKS cluster")))
			})

			It("Add node groups to the control-plane only cluster", func() {
				testCaseID = 95

				var err error
				err = helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				err = helper.ModifyEKSNodegroupOnAWS(region, clusterName, "ranchernodes", "delete", "--wait")
				Expect(err).To(BeNil())

				cluster, err = helper.ImportEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, region)
				Expect(err).To(BeNil())
				Eventually(func() bool {
					cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
					Expect(err).To(BeNil())
					return cluster.State == "waiting"
				}, "5m", "15s").Should(BeTrue())

				cluster.EKSConfig = cluster.EKSStatus.UpstreamSpec

				By("adding a NodeGroup", func() {
					cluster, err = helper.AddNodeGroup(cluster, 1, ctx.RancherAdminClient, false, true)
					Expect(err).To(BeNil())
				})
			})

			It("Fail to update both Public/Private access as false and invalid values of the access", func() {
				testCaseID = 103 // also covers 102
				invalidEndpointCheck(cluster, ctx.RancherAdminClient)
				invalidAccessValuesCheck(cluster, ctx.RancherAdminClient)
			})
		})
	})
})
