package p1_test

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var _ = BeforeEach(func() {
		// assigning cluster nil value so that every new test has a fresh value of the variable
		// this is to avoid using residual value of a cluster in a test that does not use it
		cluster = nil

		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "", false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
				err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	Context("Provisioning a cluster with invalid config", func() {

		It("should fail to provision a cluster when creating cluster with invalid name", func() {
			testCaseID = 36
			var err error
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, "@!invalid-gke-name-@#", ctx.CloudCredID, k8sVersion, zone, "", project, nil)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("InvalidFormat"))
		})

		It("User should not be able to add cluster with invalid GKE creds in Rancher", func() {
			testCaseID = 2
			invalidCredCheck(cluster, ctx.RancherAdminClient)
		})

		It("User should not be able to add a cluster using an expired GKE creds", func() {
			testCaseID = 6
			expiredCredCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to provision a cluster with invalid nodepool name", func() {
			testCaseID = 37

			updateFunc := func(clusterConfig *gke.ClusterConfig) {
				for _, np := range clusterConfig.NodePools {
					*np.Name = "#@invalid-nodepoolname-$$$$"
				}
			}

			var err error
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				clusterState, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return clusterState.Transitioning == "error" && strings.Contains(clusterState.TransitioningMessage, "Invalid value for field \"node_pool.name\"")
			}, "60s", "2s").Should(BeTrue())

		})

		It("should fail to provision a cluster nodepools is nil", func() {
			testCaseID = 27

			updateFunc := func(clusterConfig *gke.ClusterConfig) {
				clusterConfig.NodePools = nil
			}

			var err error
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				clusterState, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return clusterState.Transitioning == "error" && strings.Contains(clusterState.TransitioningMessage, "Cluster.initial_node_count must be greater than zero")
			}, "60s", "2s").Should(BeTrue())

		})

		It("should fail to provision a cluster when nodepools is an empty array", func() {
			updateFunc := func(clusterConfig *gke.ClusterConfig) {
				clusterConfig.NodePools = []gke.NodePool{}
			}

			var err error
			_, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, updateFunc)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("must have at least one node pool"))
		})
	})

	It("deleting a cluster while it is in creation state should delete it from rancher and cloud console", func() {
		testCaseID = 25
		var err error
		cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, nil)
		Expect(err).To(BeNil())

		// Wait for the cluster to appear on cloud console before deleting it
		Eventually(func() bool {
			exists, err := helper.ClusterExistsOnGCloud(clusterName, project, zone)
			Expect(err).To(BeNil())
			return exists
		}, "1m", "5s").Should(BeTrue())

		err = helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		// Wait until the cluster finishes provisioning and then begins deletion process
		Eventually(func() bool {
			exists, err := helper.ClusterExistsOnGCloud(clusterName, project, zone)
			Expect(err).To(BeNil())
			return exists
		}, "10m", "10s").Should(BeFalse())

		// Keep the cluster variable as is so that there is no error in AfterEach; failed delete operation will return an empty cluster
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should be able to create a cluster with CP K8s version v-XX-1 and NP K8s version v-XX should use v-XX-1 for both CP and NP", func() {
		if helpers.SkipUpgradeTests {
			Skip(helpers.SkipUpgradeTestsLog)
		}
		testCaseID = 33

		k8sVersions, err := helper.ListSingleVariantGKEAvailableVersions(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "")
		Expect(err).To(BeNil())
		Expect(len(k8sVersions)).To(BeNumerically(">=", 2))
		npK8sVersion := k8sVersions[0]
		cpK8sVersion := k8sVersions[1]

		updateFunc := func(clusterConfig *gke.ClusterConfig) {
			for _, np := range clusterConfig.NodePools {
				*np.Version = npK8sVersion
			}
		}

		cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, cpK8sVersion, zone, "", project, updateFunc)
		Expect(err).To(BeNil())

		Expect(*cluster.GKEConfig.KubernetesVersion).To(Equal(cpK8sVersion))
		for _, np := range *cluster.GKEConfig.NodePools {
			Expect(*np.Version).To(Equal(cpK8sVersion))
		}

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			GinkgoLogr.Info("Waiting for the k8s upgrade to appear in GKEStatus.UpstreamSpec...")
			clusterState, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			if *clusterState.GKEStatus.UpstreamSpec.KubernetesVersion != cpK8sVersion {
				return false
			}

			for _, np := range *clusterState.GKEStatus.UpstreamSpec.NodePools {
				if *np.Version != cpK8sVersion {
					return false
				}
			}
			return true
		}, "5m", "5s").Should(BeTrue(), "Failed while waiting for k8s upgrade.")
	})

	When("a cluster is created", func() {

		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("recreating a cluster while it is being deleted should recreate the cluster", func() {
			testCaseID = 26

			err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			// Wait until the cluster begins deletion process before recreating
			Eventually(func() bool {
				exists, err := helper.ClusterExistsOnGCloud(clusterName, project, zone)
				Expect(err).To(BeNil())
				return exists
			}, "1m", "5s").Should(BeFalse())

			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, nil)
			Expect(err).To(BeNil())

			// wait until the error is visible on the provisioned cluster
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.State == "provisioning" && cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "a cluster in GKE exists with the same name")
			}, "30s", "2s").Should(BeTrue())

			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should be able to update mutable parameter loggingService and monitoringService", func() {
			testCaseID = 28
			By("disabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(cluster, ctx.RancherAdminClient, "none", "none")
			})
			By("enabling the services", func() {
				updateLoggingAndMonitoringServiceCheck(cluster, ctx.RancherAdminClient, "monitoring.googleapis.com/kubernetes", "logging.googleapis.com/kubernetes")
			})
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 29
			By("enabling autoscaling", func() {
				updateAutoScaling(cluster, ctx.RancherAdminClient, true)
			})
			By("disabling autoscaling", func() {
				updateAutoScaling(cluster, ctx.RancherAdminClient, false)
			})
		})

		It("should successfully add a windows nodepool", func() {
			testCaseID = 30
			var err error
			if helpers.SkipUpgradeTests {
				Skip(helpers.SkipUpgradeTestsLog)
			}

			_, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "WINDOWS_LTSC_CONTAINERD", true, true)
			Expect(err).To(BeNil())
		})

		It("updating a cluster to all windows nodepool should fail", func() {
			testCaseID = 263

			_, err := helper.UpdateCluster(cluster, ctx.RancherAdminClient, func(upgradedCluster *management.Cluster) {
				updateNodePoolsList := *cluster.GKEConfig.NodePools
				for i := 0; i < len(updateNodePoolsList); i++ {
					updateNodePoolsList[i].Config.ImageType = "WINDOWS_LTSC_CONTAINERD"
				}

				upgradedCluster.GKEConfig.NodePools = &updateNodePoolsList
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least 1 Linux node pool is required"))
		})

		It("should be able to update combination mutable parameter", func() {
			testCaseID = 31
			combinationMutableParameterUpdate(cluster, ctx.RancherAdminClient)
		})

		It("should successfully update with new cloud credentials", func() {
			testCaseID = 5
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})
	})

	When("creating a cluster with at least 2 nodepools", func() {
		BeforeEach(func() {
			var err error
			updateFunc := func(clusterConfig *gke.ClusterConfig) {
				var updateNodePoolsList []gke.NodePool
				npTemplate := clusterConfig.NodePools[0]

				for i := 0; i < 2; i++ {
					newNodePool := gke.NodePool{
						Autoscaling:       npTemplate.Autoscaling,
						Config:            npTemplate.Config,
						InitialNodeCount:  npTemplate.InitialNodeCount,
						Management:        npTemplate.Management,
						MaxPodsConstraint: npTemplate.MaxPodsConstraint,
						Name:              pointer.String(namegen.AppendRandomString(*npTemplate.Name)),
						Version:           pointer.String(k8sVersion),
					}
					updateNodePoolsList = append(updateNodePoolsList, newNodePool)
				}
				clusterConfig.NodePools = updateNodePoolsList
			}
			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, updateFunc)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("for a given NodePool with a non-windows imageType, updating it to a windows imageType should fail", func() {
			testCaseID = 34
			var err error
			cluster, err = helper.UpdateCluster(cluster, ctx.RancherAdminClient, func(upgradedCluster *management.Cluster) {
				updateNodePoolsList := *cluster.GKEConfig.NodePools
				updateNodePoolsList[0].Config.ImageType = "WINDOWS_LTSC_CONTAINERD"

				upgradedCluster.GKEConfig.NodePools = &updateNodePoolsList
			})

			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "Node pools cannot be upgraded between Windows and non-Windows image families")
			}, "30s", "2s").Should(BeTrue())
		})
	})

	When("a cluster is created for upgrade scenarios", func() {

		BeforeEach(func() {
			if helpers.SkipUpgradeTests {
				Skip(helpers.SkipUpgradeTestsLog)
			}

			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "", true)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("Using kubernetes version %s for cluster %s", k8sVersion, clusterName))

			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully update a cluster while it is still in updating state", func() {
			testCaseID = 35
			updateClusterInUpdatingState(cluster, ctx.RancherAdminClient)
		})

		It("should successfully upgrade CP & NP version simultaneously", func() {
			upgradeK8sVersionChecks(cluster, ctx.RancherAdminClient)
		})
	})

	When("a private cluster is created", func() {
		var err error

		BeforeEach(func() {
			updateFunc := func(clusterConfig *gke.ClusterConfig) {
				network := pointer.String("hosted-providers-ci-private")
				clusterConfig.Network = network
				clusterConfig.Subnetwork = network
				clusterConfig.PrivateClusterConfig.EnablePrivateNodes = true
				clusterConfig.PrivateClusterConfig.MasterIpv4CidrBlock = fmt.Sprintf("172.16.%d.0/28", rand.Intn(10))

				currentSpec := CurrentSpecReport().FullText()
				if strings.Contains(currentSpec, "MasterAuthorizedNetworks") {
					_, authorizedIP, err := net.ParseCIDR(helpers.GetRancherIP() + "/32")
					Expect(err).To(BeNil())

					newCidrBlock := gke.CidrBlock{
						CidrBlock:   authorizedIP.String(),
						DisplayName: "Rancher",
					}
					clusterConfig.MasterAuthorizedNetworksConfig.Enabled = true
					clusterConfig.MasterAuthorizedNetworksConfig.CidrBlocks = append(clusterConfig.MasterAuthorizedNetworksConfig.CidrBlocks, newCidrBlock)
				}
			}

			cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, updateFunc)
			Expect(err).To(BeNil())

			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			Expect(cluster.GKEConfig.PrivateClusterConfig.EnablePrivateNodes).To(BeTrue())
			Expect(cluster.GKEStatus.UpstreamSpec.PrivateClusterConfig.EnablePrivateNodes).To(BeTrue())
		})

		It("should successfully create with public endpoint", func() {
			testCaseID = 22

			cluster, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "", true, true)
			Expect(err).To(BeNil())
		})

		It("should successfully create with public endpoint and MasterAuthorizedNetworks", func() {
			testCaseID = 24

			Expect(cluster.GKEConfig.MasterAuthorizedNetworksConfig.Enabled).To(BeTrue())
			Expect(cluster.GKEStatus.UpstreamSpec.MasterAuthorizedNetworksConfig.Enabled).To(BeTrue())

			cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, 1, false, true)
			Expect(err).To(BeNil())

		})
	})
})
