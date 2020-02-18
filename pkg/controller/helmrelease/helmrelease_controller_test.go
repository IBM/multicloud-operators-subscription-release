// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helmrelease

import (
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	appv1alpha1 "github.com/IBM/multicloud-operators-subscription-release/pkg/apis/app/v1alpha1"
)

var (
	helmReleaseNS = "kube-system"
)

func TestReconcile(t *testing.T) {
	defer klog.Flush()

	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.

	t.Log("Create manager")

	mgr, err := manager.New(cfg, manager.Options{
		MetricsBindAddress: "0",
		LeaderElection:     false,
	})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	c := mgr.GetClient()

	rec := &ReconcileHelmRelease{
		mgr,
	}

	t.Log("Setup test reconcile")
	g.Expect(Add(mgr)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	//
	//Github succeed
	//
	t.Log("Github succeed test")

	helmReleaseName := "example-github-succeed"
	helmReleaseKey := types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance := &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	instanceResp := &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceResp)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(instanceResp.Status.DeployedRelease).NotTo(gomega.BeNil())

	//
	//Github failed
	//
	t.Log("Github failed test")

	helmReleaseName = "example-github-failed"
	helmReleaseKey = types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance = &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "wrong path",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(2 * time.Second)

	instanceResp = &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceResp)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(instanceResp.Status.DeployedRelease).To(gomega.BeNil())

	//
	//helmRepo succeeds
	//
	t.Log("helmrepo succeed test")

	helmReleaseName = "example-helmrepo-succeed"
	helmReleaseKey = types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance = &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.HelmRepoSourceType,
				HelmRepo: &appv1alpha1.HelmRepo{
					Urls: []string{"https://raw.github.com/IBM/multicloud-operators-subscription-release/master/test/helmrepo/subscription-release-test-1-0.1.0.tgz"},
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	instanceResp = &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceResp)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(instanceResp.Status.DeployedRelease).NotTo(gomega.BeNil())

	//
	//helmRepo failure
	//
	t.Log("Github failure test")

	helmReleaseName = "example-helmrepo-failure"
	helmReleaseKey = types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance = &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.HelmRepoSourceType,
				HelmRepo: &appv1alpha1.HelmRepo{
					Urls: []string{"https://raw.github.com/IBM/multicloud-operators-subscription-release/wrongurl"},
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	instanceResp = &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceResp)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(instanceResp.Status.DeployedRelease).To(gomega.BeNil())

	//
	//Github succeed create-delete
	//
	t.Log("Github succeed create-delete test")

	helmReleaseName = "example-github-delete"
	helmReleaseKey = types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance = &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	//Creation
	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	instanceRespCD := &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceRespCD)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	g.Expect(instanceRespCD.Status.DeployedRelease).NotTo(gomega.BeNil())

	//Deletion
	err = c.Delete(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(8 * time.Second)

	instanceRespDel := &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceRespDel)
	g.Expect(err).To(gomega.HaveOccurred())

	time.Sleep(2 * time.Second)

	//
	//Github succeed create-update
	//
	t.Log("Github succeed create-update")

	helmReleaseName = "example-github-update"
	helmReleaseKey = types.NamespacedName{
		Name:      helmReleaseName,
		Namespace: helmReleaseNS,
	}
	instance = &appv1alpha1.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmRelease",
			APIVersion: "app.ibm.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	//Creation
	t.Log("Github succeed create-update -> CR create")

	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	t.Log("Github succeed create-update -> CR get response")

	instanceRespCU := &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceRespCU)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(2 * time.Second)

	g.Expect(instanceRespCU.Status.DeployedRelease).NotTo(gomega.BeNil())

	//Update
	t.Log("Github succeed create-update -> CR get")

	err = c.Get(context.TODO(), helmReleaseKey, instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	var spec interface{}

	yaml.Unmarshal([]byte("l1:v1"), &spec)
	instance.Spec = spec

	t.Log("Github succeed create-update -> CR update")

	err = c.Update(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(4 * time.Second)

	t.Log("Github succeed create-update -> CR get response")

	instanceRespUp := &appv1alpha1.HelmRelease{}
	err = c.Get(context.TODO(), helmReleaseKey, instanceRespUp)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// TestNewManager
	helmReleaseName = "test-new-manager"

	instance = &appv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	assert.NoError(t, err)

	time.Sleep(6 * time.Second)

	_, err = rec.newHelmReleaseManager(instance)
	assert.NoError(t, err)

	// TestNewManagerShortReleaseName
	helmReleaseName = "test-new-manager-short-release-name"
	instance = &appv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	assert.NoError(t, err)

	time.Sleep(6 * time.Second)

	_, err = rec.newHelmReleaseManager(instance)
	assert.NoError(t, err)

	// TestNewManagerValues
	helmReleaseName = "test-new-manager-values"
	instance = &appv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	yaml.Unmarshal([]byte("l1:v1"), &spec)
	instance.Spec = spec

	//Values well formed
	err = c.Create(context.TODO(), instance)
	assert.NoError(t, err)

	time.Sleep(6 * time.Second)

	// TestNewManagerErrors
	helmReleaseName = "test-new-manager-errors"

	instance = &appv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmReleaseName,
			Namespace: helmReleaseNS,
		},
		Repo: appv1alpha1.HelmReleaseRepo{
			Source: &appv1alpha1.Source{
				SourceType: appv1alpha1.GitHubSourceType,
				GitHub: &appv1alpha1.GitHub{
					Urls:      []string{"https://github.com/IBM/multicloud-operators-subscription-release.git"},
					ChartPath: "test/github/subscription-release-test-1",
				},
			},
			ChartName: "subscription-release-test-1",
		},
	}

	err = c.Create(context.TODO(), instance)
	assert.NoError(t, err)

	time.Sleep(2 * time.Second)
}
