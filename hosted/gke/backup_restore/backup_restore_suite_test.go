/*
Copyright © 2023 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backup_restore_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	. "github.com/rancher-sandbox/qase-ginkgo"

	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy          = 1
	backupResourceName  = "hp-backup"
	restoreResourceName = "hp-restore"
)

var (
	testCaseID              int64
	clusterName, backupFile string
	ctx                     helpers.RancherContext
	cluster                 *management.Cluster
	project                 = helpers.GetGKEProjectID()
	zone                    = helpers.GetGKEZone()
	k3sVersion              = os.Getenv("INSTALL_K3S_VERSION")
)

func TestBackupRestore(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.CommonSynchronizedBeforeSuite()
	ctx = helpers.CommonBeforeSuite()
	RunSpecs(t, "BackupRestore Suite")
}

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

var _ = BeforeEach(func() {
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "", false)
	Expect(err).NotTo(HaveOccurred())
	GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

	if helpers.IsImport {
		By("importing the cluster")
		err = helper.CreateGKEClusterOnGCloud(zone, clusterName, project, k8sVersion)
		Expect(err).To(BeNil())
		cluster, err = helper.ImportGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, zone, project)
		Expect(err).To(BeNil())
	} else {
		By("provisioning the cluster")
		cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, "", project, nil)
		Expect(err).To(BeNil())
	}
	cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
	Expect(err).To(BeNil())
})

var _ = AfterEach(func() {
	if ctx.ClusterCleanup && cluster != nil {
		err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		if helpers.IsImport {
			err = helper.DeleteGKEClusterOnGCloud(zone, project, clusterName)
			Expect(err).To(BeNil())
		}
	} else {
		fmt.Println("Skipping downstream cluster deletion: ", clusterName)
	}
})

func restoreNodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)
	configNodePools := *cluster.GKEConfig.NodePools
	initialNodeCount := *configNodePools[0].InitialNodeCount

	By("scaling up the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("adding a nodepool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, client, increaseBy, "", true, true)
		Expect(err).To(BeNil())
	})
}

func BackupRestoreChecks(k *kubectl.Kubectl) {
	By("Checking hosted cluster is ready", func() {
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	By("Performing a backup", func() {
		backupFile = helpers.ExecuteBackup(k, backupResourceName)
	})

	By("Perform restore pre-requisites: Uninstalling k3s", func() {
		out, err := exec.Command("k3s-uninstall.sh").CombinedOutput()
		Expect(err).To(Not(HaveOccurred()), out)
	})

	By("Perform restore pre-requisites: Getting k3s ready", func() {
		helpers.InstallK3S(k, k3sVersion, "none", "none")
	})

	By("Performing a restore", func() {
		helpers.ExecuteRestore(k, restoreResourceName, backupFile)
	})

	By("Performing post migration installations: Installing CertManager", func() {
		helpers.InstallCertManager(k, "none", "none")
	})

	By("Performing post migration installations: Installing Rancher Manager", func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "none", "none")
	})

	By("Performing post migration installations: Checking Rancher Deployments", func() {
		helpers.CheckRancherDeployments(k)
	})

	By("Checking hosted cluster can be modified", func() {
		restoreNodesChecks(cluster, ctx.RancherAdminClient, clusterName)
	})
}
