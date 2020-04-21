/*
Copyright 2019 The Kubernetes Authors.

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

package helmrelease

import (
	"context"
	"io/ioutil"
	"os"

	helmrelease "github.com/operator-framework/operator-sdk/pkg/helm/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appv1 "github.com/open-cluster-management/multicloud-operators-subscription-release/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-subscription-release/pkg/utils"
)

//newHelmReleaseManagerFactory create a new manager returns a helmManagerFactory
func (r *ReconcileHelmRelease) newHelmReleaseManagerFactory(
	s *appv1.HelmRelease) (helmrelease.ManagerFactory, error) {
	configMap, err := utils.GetConfigMap(r.GetClient(), s.Namespace, s.Repo.ConfigMapRef)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	secret, err := utils.GetSecret(r.GetClient(), s.Namespace, s.Repo.SecretRef)
	if err != nil {
		klog.Error(err, " - Failed to retrieve secret ", s.Repo.SecretRef.Name)
		return nil, err
	}

	chartsDir := os.Getenv(appv1.ChartsDir)
	if chartsDir == "" {
		chartsDir, err = ioutil.TempDir("/tmp", "charts")
		if err != nil {
			klog.Error(err, " - Can not create tempdir")
			return nil, err
		}
	}

	chartDir, err := utils.DownloadChart(configMap, secret, chartsDir, s)
	klog.V(3).Info("ChartDir: ", chartDir)

	if err != nil {
		klog.Error(err, " - Failed to download the chart")
		return nil, err
	}

	f := helmrelease.NewManagerFactory(r.Manager, chartDir)

	return f, nil
}

//newHelmReleaseManager create a new manager returns a helmManager
func (r *ReconcileHelmRelease) newHelmReleaseManager(
	s *appv1.HelmRelease, request reconcile.Request, factory helmrelease.ManagerFactory) (helmrelease.Manager, error) {
	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(s.GroupVersionKind())
	o.SetNamespace(request.Namespace)
	o.SetName(request.Name)

	err := r.GetClient().Get(context.TODO(), request.NamespacedName, o)
	if err != nil {
		klog.Error(err, " - Failed to lookup resource")
		return nil, err
	}

	manager, err := factory.NewManager(o, nil)
	if err != nil {
		klog.Error(err, " - Failed to get release manager")
		return nil, err
	}

	return manager, nil
}
