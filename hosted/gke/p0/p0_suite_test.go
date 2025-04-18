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

package p0_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

const (
	increaseBy = 1
)

var (
	ctx                                helpers.RancherContext
	cluster                            *management.Cluster
	clusterName, zone, region, project string
	testCaseID                         int64
	updateFunc                         func(clusterConfig *gke.ClusterConfig)
)

func TestP0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "P0 Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	helpers.CommonSynchronizedBeforeSuite()
	return nil
}, func() {
	ctx = helpers.CommonBeforeSuite()
})

var _ = BeforeEach(func() {
	// Setting this to nil ensures we do not use the `cluster` variable value from another test running in parallel with this one.
	cluster = nil
	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)
	zone = helpers.GetGKEZone()
	region = helpers.GetGKERegion()
	project = helpers.GetGKEProjectID()
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

func p0upgradeK8sVersionChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)

	versions, err := helper.ListGKEAvailableVersions(client, cluster.ID)
	Expect(err).To(BeNil())
	Expect(versions).ToNot(BeEmpty())
	upgradeToVersion := versions[0]
	GinkgoLogr.Info(fmt.Sprintf("Upgrading cluster to GKE version %s", upgradeToVersion))

	// Upgrading controlplane and nodepool sequentially
	By("upgrading the ControlPlane", func() {
		cluster, err = helper.UpgradeKubernetesVersion(cluster, upgradeToVersion, client, false, true, true)
		Expect(err).To(BeNil())
	})

	By("upgrading the Nodepools", func() {
		cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, client, true, true)
		Expect(err).To(BeNil())
	})
}

func p0NodesChecks(cluster *management.Cluster, client *rancher.Client, clusterName string) {
	helpers.ClusterIsReadyChecks(cluster, client, clusterName)
	configNodePools := *cluster.GKEConfig.NodePools
	initialNodeCount := *configNodePools[0].InitialNodeCount

	By("scaling up the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("scaling down the nodepool", func() {
		var err error
		cluster, err = helper.ScaleNodePool(cluster, client, initialNodeCount, true, true)
		Expect(err).To(BeNil())
	})

	By("adding a nodepool", func() {
		var err error
		cluster, err = helper.AddNodePool(cluster, client, increaseBy, "", true, true)
		Expect(err).To(BeNil())
	})

	By("deleting the nodepool", func() {
		var err error
		cluster, err = helper.DeleteNodePool(cluster, client, true, true)
		Expect(err).To(BeNil())
	})
}
