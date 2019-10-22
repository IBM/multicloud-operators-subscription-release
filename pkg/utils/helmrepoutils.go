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

package utils

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	appv1alpha1 "github.com/IBM/multicloud-operators-subscription-release/pkg/apis/app/v1alpha1"
)

var log = logf.Log.WithName("utils")

//GetHelmRepoClient returns an *http.client to access the helm repo
func GetHelmRepoClient(parentNamespace string, configMap *corev1.ConfigMap) (*http.Client, error) {
	srLogger := log.WithValues("package", "utils", "method", "GetHelmRepoClient")

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	if configMap != nil {
		configData := configMap.Data
		srLogger.Info("ConfigRef retrieved", "configMap.Data", configData)
		insecureSkipVerify := configData["insecureSkipVerify"]

		if insecureSkipVerify != "" {
			b, err := strconv.ParseBool(insecureSkipVerify)
			if err != nil {
				if errors.IsNotFound(err) {
					return nil, nil
				}

				srLogger.Error(err, "Unable to parse", "insecureSkipVerify", insecureSkipVerify)

				return nil, err
			}

			srLogger.Info("Set InsecureSkipVerify", "insecureSkipVerify", b)
			transport.TLSClientConfig.InsecureSkipVerify = b
		} else {
			srLogger.Info("insecureSkipVerify is not specified")
		}
	} else {
		srLogger.Info("configMap is nil")
	}

	httpClient := http.DefaultClient
	httpClient.Transport = transport
	srLogger.Info("InsecureSkipVerify equal", "InsecureSkipVerify", transport.TLSClientConfig.InsecureSkipVerify)

	return httpClient, nil
}

//GetConfigMap search the config map containing the helm repo client configuration.
func GetConfigMap(client client.Client, parentNamespace string, configMapRef *corev1.ObjectReference) (configMap *corev1.ConfigMap, err error) {
	srLogger := log.WithValues("package", "utils", "method", "getConfigMap")

	if configMapRef != nil {
		srLogger.Info("Retrieve configMap ", "parentNamespace", parentNamespace, "configMapRef.Name", configMapRef.Name)
		ns := configMapRef.Namespace

		if ns == "" {
			ns = parentNamespace
		}

		configMap = &corev1.ConfigMap{}

		err = client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: configMapRef.Name}, configMap)
		if err != nil {
			if errors.IsNotFound(err) {
				srLogger.Error(err, "ConfigMap not found ", "Name:", configMapRef.Name, " on namespace: ", ns)
				return nil, nil
			}

			srLogger.Error(err, "Failed to get configMap ", "Name:", configMapRef.Name, " on namespace: ", ns)

			return nil, err
		}

		srLogger.Info("ConfigMap found ", "Name:", configMapRef.Name, " on namespace: ", ns)
	} else {
		srLogger.Info("no configMapRef defined ", "parentNamespace", parentNamespace)
	}

	return configMap, err
}

//GetSecret returns the secret to access the helm-repo
func GetSecret(client client.Client, parentNamespace string, secretRef *corev1.ObjectReference) (secret *corev1.Secret, err error) {
	srLogger := log.WithValues("package", "utils", "method", "getSecret")

	if secretRef != nil {
		srLogger.Info("retrieve secret", "parentNamespace", parentNamespace, "secretRef", secretRef)

		ns := secretRef.Namespace
		if ns == "" {
			ns = parentNamespace
		}

		secret = &corev1.Secret{}

		err = client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: secretRef.Name}, secret)
		if err != nil {
			srLogger.Error(err, "Failed to get secret ", "Name:", secretRef.Name, " on namespace: ", secretRef.Namespace)
			return nil, err
		}

		srLogger.Info("Secret found ", "Name:", secretRef.Name, " on namespace: ", secretRef.Namespace)
	} else {
		srLogger.Info("No secret defined", "parentNamespace", parentNamespace)
	}

	return secret, err
}

func DownloadChart(configMap *corev1.ConfigMap, secret *corev1.Secret, chartsDir string, s *appv1alpha1.HelmRelease) (chartDir string, err error) {
	switch strings.ToLower(string(s.Spec.Source.SourceType)) {
	case string(appv1alpha1.HelmRepoSourceType):
		return DownloadChartFromHelmRepo(configMap, secret, chartsDir, s)
	case string(appv1alpha1.GitHubSourceType):
		return DownloadChartFromGitHub(configMap, secret, chartsDir, s)
	default:
		return "", fmt.Errorf("sourceType '%s' unsupported", s.Spec.Source.SourceType)
	}
}

//DownloadChartFromGitHub downloads a chart into the charsDir
func DownloadChartFromGitHub(configMap *corev1.ConfigMap, secret *corev1.Secret, chartsDir string, s *appv1alpha1.HelmRelease) (chartDir string, err error) {
	srLogger := log.WithValues("HelmRelease.Namespace", s.Namespace, "SubscrptionRelease.Name", s.Name)

	if s.Spec.Source.GitHub == nil {
		err := fmt.Errorf("github type but Spec.GitHub is not defined")
		return "", err
	}

	if _, err := os.Stat(chartsDir); os.IsNotExist(err) {
		err := os.MkdirAll(chartsDir, 0755)
		if err != nil {
			srLogger.Error(err, "Unable to create chartDir: ", "chartsDir", chartsDir)
			return "", err
		}
	}

	destRepo := filepath.Join(chartsDir, s.Spec.ReleaseName, s.Namespace, s.Spec.ChartName)

	for _, url := range s.Spec.Source.GitHub.Urls {
		options := &git.CloneOptions{
			URL:               url,
			Depth:             1,
			SingleBranch:      true,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		}

		if secret != nil && secret.Data != nil {
			srLogger.Info("Add credentials")

			options.Auth = &githttp.BasicAuth{
				Username: string(secret.Data["user"]),
				Password: string(secret.Data["password"]),
			}
		}

		if s.Spec.Source.GitHub.Branch == "" {
			options.ReferenceName = plumbing.Master
		} else {
			options.ReferenceName = plumbing.ReferenceName(s.Spec.Source.GitHub.Branch)
		}

		os.RemoveAll(chartDir)

		_, err = git.PlainClone(destRepo, false, options)
		if err != nil {
			os.RemoveAll(destRepo)
			srLogger.Error(err, "Clone failed", "url", url)

			continue
		}
	}

	if err != nil {
		srLogger.Error(err, "All urls failed")
	}

	chartDir = filepath.Join(destRepo, s.Spec.Source.GitHub.ChartPath)

	return chartDir, err
}

//DownloadChartFromHelmRepo downloads a chart into the charsDir
func DownloadChartFromHelmRepo(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	chartsDir string,
	s *appv1alpha1.HelmRelease) (chartDir string, err error) {
	srLogger := log.WithValues("HelmRelease.Namespace", s.Namespace, "SubscrptionRelease.Name", s.Name)

	if s.Spec.Source.HelmRepo == nil {
		err := fmt.Errorf("helmrepo type but Spec.HelmRepo is not defined")
		return "", err
	}

	if _, err := os.Stat(chartsDir); os.IsNotExist(err) {
		err := os.MkdirAll(chartsDir, 0755)
		if err != nil {
			srLogger.Error(err, "Unable to create chartDir: ", "chartsDir", chartsDir)
			return "", err
		}
	}

	httpClient, err := GetHelmRepoClient(s.Namespace, configMap)
	if err != nil {
		srLogger.Error(err, "Failed to create httpClient ", "sr.Spec.SecretRef.Name", s.Spec.SecretRef.Name)
		return "", err
	}

	var downloadErr error

	for _, urlelem := range s.Spec.Source.HelmRepo.Urls {
		var URLP *url.URL

		URLP, downloadErr = url.Parse(urlelem)
		if downloadErr != nil {
			srLogger.Error(downloadErr, "url", urlelem)
			continue
		}

		fileName := filepath.Base(URLP.Path)
		// Create the file
		chartZip := filepath.Join(chartsDir, fileName)
		if _, err := os.Stat(chartZip); os.IsNotExist(err) {
			var req *http.Request

			req, downloadErr = http.NewRequest(http.MethodGet, urlelem, nil)
			if downloadErr != nil {
				srLogger.Error(downloadErr, "Can not build request: ", "urlelem", urlelem)
				continue
			}

			if secret != nil && secret.Data != nil {
				req.SetBasicAuth(string(secret.Data["user"]), string(secret.Data["password"]))
			}

			var resp *http.Response

			resp, downloadErr = httpClient.Do(req)
			if downloadErr != nil {
				srLogger.Error(downloadErr, "Http request failed: ", "urlelem", urlelem)
				continue
			}

			srLogger.Info("Get succeeded: ", "urlelem", urlelem)

			defer resp.Body.Close()

			var out *os.File

			out, downloadErr = os.Create(chartZip)
			if downloadErr != nil {
				srLogger.Error(downloadErr, "Failed to create: ", "chartZip", chartZip)
				continue
			}

			defer out.Close()

			// Write the body to file
			_, downloadErr = io.Copy(out, resp.Body)
			if downloadErr != nil {
				srLogger.Error(downloadErr, "Failed to copy body: ", "chartZip", chartZip)
				continue
			}
		}

		var r *os.File

		r, downloadErr = os.Open(chartZip)
		if downloadErr != nil {
			srLogger.Error(downloadErr, "Failed to open: ", "chartZip", chartZip)
			continue
		}

		chartDirUnzip := filepath.Join(chartsDir, s.Spec.ReleaseName, s.Namespace)
		chartDir = filepath.Join(chartDirUnzip, s.Spec.ChartName)
		//Clean before untar
		os.RemoveAll(chartDirUnzip)

		downloadErr = Untar(chartDirUnzip, r)
		if downloadErr != nil {
			//Remove zip because failed to untar and so probably corrupted
			os.RemoveAll(chartZip)
			srLogger.Error(downloadErr, "Failed to unzip: ", "chartZip", chartZip)

			continue
		}
	}

	return chartDir, downloadErr
}

//DownloadGitHubRepo downloads a github repo into the charsDir
func DownloadGitHubRepo(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	chartsDir string,
	s *appv1alpha1.HelmChartSubscription) (destRepo string, commitID string, err error) {
	srLogger := log.WithValues("HelmRelease.Namespace", s.Namespace, "SubscrptionRelease.Name", s.Name)

	if s.Spec.Source.GitHub == nil {
		err := fmt.Errorf("github type but Spec.GitHub is not defined")
		return "", "", err
	}

	if _, err := os.Stat(chartsDir); os.IsNotExist(err) {
		err := os.MkdirAll(chartsDir, 0755)
		if err != nil {
			srLogger.Error(err, "Unable to create chartDir: ", "chartsDir", chartsDir)
			return "", "", err
		}
	}

	destRepo = filepath.Join(chartsDir, s.Name, s.Namespace)

	for _, url := range s.Spec.Source.GitHub.Urls {
		options := &git.CloneOptions{
			URL:               url,
			Depth:             1,
			SingleBranch:      true,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		}

		if secret != nil && secret.Data != nil {
			srLogger.Info("Add credentials")

			options.Auth = &githttp.BasicAuth{
				Username: string(secret.Data["user"]),
				Password: string(secret.Data["password"]),
			}
		}

		if s.Spec.Source.GitHub.Branch == "" {
			options.ReferenceName = plumbing.Master
		} else {
			options.ReferenceName = plumbing.ReferenceName(s.Spec.Source.GitHub.Branch)
		}

		os.RemoveAll(destRepo)

		r, errClone := git.PlainClone(destRepo, false, options)

		if errClone != nil {
			os.RemoveAll(destRepo)
			srLogger.Error(errClone, "Clone failed", "url", url)
			err = errClone

			continue
		}

		h, errHead := r.Head()

		if errHead != nil {
			os.RemoveAll(destRepo)
			srLogger.Error(errHead, "Get Head failed", "url", url)
			err = errHead

			continue
		}

		commitID = h.Hash().String()
		srLogger.Info("commitID", "commitID", commitID)
	}

	if err != nil {
		srLogger.Error(err, "All urls failed")
	}

	return destRepo, commitID, err
}

//Untar untars the reader into the dst directory
func Untar(dst string, r io.Reader) error {
	srLogger := log.WithValues("destination", dst)

	gzr, err := gzip.NewReader(r)
	if err != nil {
		srLogger.Error(err, "")
		return err
	}

	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF: // if no more files are found return
			return nil
		case err != nil: // return any other error
			srLogger.Error(err, "")
			return err
		case header == nil: // if the header is nil, just skip it (not sure how this happens)
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {
		case tar.TypeDir: // if its a dir and it doesn't exist create it
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					srLogger.Error(err, "")
					return err
				}
			}
		case tar.TypeReg: // if it's a file create it
			dir := filepath.Dir(target)
			if _, err := os.Stat(dir); err != nil {
				if err := os.MkdirAll(dir, 0755); err != nil {
					srLogger.Error(err, "")
					return err
				}
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				srLogger.Error(err, "")
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				srLogger.Error(err, "")
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
