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
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	appv1 "github.com/open-cluster-management/multicloud-operators-subscription-release/pkg/apis/apps/v1"
)

//GetHelmRepoClient returns an *http.client to access the helm repo
func GetHelmRepoClient(parentNamespace string, configMap *corev1.ConfigMap) (rest.HTTPClient, error) {
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
		klog.V(5).Info("ConfigRef retrieved :", configData)
		insecureSkipVerify := configData["insecureSkipVerify"]

		if insecureSkipVerify != "" {
			b, err := strconv.ParseBool(insecureSkipVerify)
			if err != nil {
				if errors.IsNotFound(err) {
					return nil, nil
				}

				klog.Error(err, " - Unable to parse insecureSkipVerify", insecureSkipVerify)

				return nil, err
			}

			klog.V(5).Info("Set InsecureSkipVerify: ", b)
			transport.TLSClientConfig.InsecureSkipVerify = b
		} else {
			klog.V(5).Info("insecureSkipVerify is not specified")
		}
	} else {
		klog.V(5).Info("configMap is nil")
	}

	httpClient := http.DefaultClient
	httpClient.Transport = transport
	klog.V(5).Info("InsecureSkipVerify equal ", transport.TLSClientConfig.InsecureSkipVerify)

	return httpClient, nil
}

//DownloadChart downloads the charts
func DownloadChart(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	chartsDir string,
	s *appv1.HelmRelease) (chartDir string, err error) {
	destRepo := filepath.Join(chartsDir, s.Name, s.Namespace, s.Repo.ChartName)
	if _, err := os.Stat(destRepo); os.IsNotExist(err) {
		err := os.MkdirAll(destRepo, 0750)
		if err != nil {
			klog.Error(err, " - Unable to create chartDir: ", destRepo)
			return "", err
		}
	}

	switch strings.ToLower(string(s.Repo.Source.SourceType)) {
	case string(appv1.HelmRepoSourceType):
		return DownloadChartFromHelmRepo(configMap, secret, destRepo, s)
	case string(appv1.GitHubSourceType):
		return DownloadChartFromGitHub(configMap, secret, destRepo, s)
	default:
		return "", fmt.Errorf("sourceType '%s' unsupported", s.Repo.Source.SourceType)
	}
}

//DownloadChartFromGitHub downloads a chart into the charsDir
func DownloadChartFromGitHub(configMap *corev1.ConfigMap, secret *corev1.Secret, destRepo string, s *appv1.HelmRelease) (chartDir string, err error) {
	if s.Repo.Source.GitHub == nil {
		err := fmt.Errorf("github type but Spec.GitHub is not defined")
		return "", err
	}

	_, err = DownloadGitHubRepo(configMap, secret, destRepo, s.Repo.Source.GitHub.Urls, s.Repo.Source.GitHub.Branch)

	if err != nil {
		return "", err
	}

	chartDir = filepath.Join(destRepo, s.Repo.Source.GitHub.ChartPath)

	return chartDir, err
}

//DownloadGitHubRepo downloads a github repo into the charsDir
func DownloadGitHubRepo(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	destRepo string,
	urls []string, branch string) (commitID string, err error) {
	for _, url := range urls {
		options := &git.CloneOptions{
			URL:               url,
			Depth:             1,
			SingleBranch:      true,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		}

		if secret != nil && secret.Data != nil {
			klog.V(5).Info("Add credentials")

			options.Auth = &githttp.BasicAuth{
				Username: string(secret.Data["user"]),
				Password: GetAccessToken(secret),
			}
		}

		if branch == "" {
			options.ReferenceName = plumbing.Master
		} else {
			options.ReferenceName = plumbing.ReferenceName("refs/heads/" + branch)
		}

		rErr := os.RemoveAll(destRepo)
		if rErr != nil {
			klog.Error(err, "- Failed to remove all: ", destRepo)
		}

		r, errClone := git.PlainClone(destRepo, false, options)

		if errClone != nil {
			rErr = os.RemoveAll(destRepo)
			if rErr != nil {
				klog.Error(err, "- Failed to remove all: ", destRepo)
			}

			klog.Error(errClone, " - Clone failed: ", url)
			err = errClone

			continue
		}

		h, errHead := r.Head()

		if errHead != nil {
			rErr := os.RemoveAll(destRepo)
			if rErr != nil {
				klog.Error(err, "- Failed to remove all: ", destRepo)
			}

			klog.Error(errHead, " - Get Head failed: ", url)
			err = errHead

			continue
		}

		commitID = h.Hash().String()
		klog.V(5).Info("commitID: ", commitID)
	}

	if err != nil {
		klog.Error(err, " - All urls failed")
	}

	return commitID, err
}

//DownloadChartFromHelmRepo downloads a chart into the chartDir
func DownloadChartFromHelmRepo(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	destRepo string,
	s *appv1.HelmRelease) (chartDir string, err error) {
	if s.Repo.Source.HelmRepo == nil {
		err := fmt.Errorf("helmrepo type but Spec.HelmRepo is not defined")
		return "", err
	}

	var urlsError string

	for _, url := range s.Repo.Source.HelmRepo.Urls {
		chartDir, err := downloadChartFromURL(configMap, secret, destRepo, s, url)
		if err == nil {
			return chartDir, nil
		}

		urlsError += " - url: " + url + " error: " + err.Error()
	}

	return "", fmt.Errorf("failed to download chart from helm repo. " + urlsError)
}

func downloadChartFromURL(configMap *corev1.ConfigMap,
	secret *corev1.Secret,
	destRepo string,
	s *appv1.HelmRelease,
	url string) (chartDir string, err error) {
	chartZip, downloadErr := downloadFile(s.Namespace, configMap, url, secret, destRepo)
	if downloadErr != nil {
		klog.Error(downloadErr, " - url: ", url)
		return "", downloadErr
	}

	r, downloadErr := os.Open(filepath.Clean(chartZip))
	if downloadErr != nil {
		klog.Error(downloadErr, " - Failed to open: ", chartZip, " using url: ", url)
		return "", downloadErr
	}

	chartDir = filepath.Join(destRepo, s.Repo.ChartName)
	chartDir = filepath.Clean(chartDir)
	//Clean before untar
	err = os.RemoveAll(chartDir)
	if err != nil {
		klog.Error(err, "- Failed to remove all: ", chartDir, " for ", chartZip, " using url: ", url)
	}

	//Untar
	err = chartutil.Expand(destRepo, r)
	if err != nil {
		//Remove zip because failed to untar and so probably corrupted
		rErr := os.RemoveAll(chartZip)
		if rErr != nil {
			klog.Error(rErr, "- Failed to remove all: ", chartZip)
		}

		klog.Error(err, "- Failed to unzip: ", chartZip, " using url: ", url)

		return "", err
	}

	return chartDir, nil
}

//downloadFile downloads a files and post it in the chartsDir.
func downloadFile(parentNamespace string, configMap *corev1.ConfigMap,
	fileURL string,
	secret *corev1.Secret,
	chartsDir string) (string, error) {
	klog.V(4).Info("fileURL: ", fileURL)

	URLP, downloadErr := url.Parse(fileURL)
	if downloadErr != nil {
		klog.Error(downloadErr, " - url:", fileURL)
		return "", downloadErr
	}

	fileName := filepath.Base(URLP.RequestURI())
	klog.V(4).Info("fileName: ", fileName)
	// Create the file
	chartZip := filepath.Join(chartsDir, fileName)
	klog.V(4).Info("chartZip: ", chartZip)

	if chartZip == chartsDir {
		downloadErr = fmt.Errorf("failed to parse fileName from fileURL %s", fileURL)
		return "", downloadErr
	}

	switch URLP.Scheme {
	case "file":
		downloadErr = downloadFileLocal(URLP, chartZip)
	case "http", "https":
		downloadErr = downloadFileHTTP(parentNamespace, configMap, fileURL, secret, chartZip)
	default:
		downloadErr = fmt.Errorf("unsupported scheme %s", URLP.Scheme)
	}

	return chartZip, downloadErr
}

func downloadFileLocal(urlP *url.URL,
	chartZip string) error {
	sourceFile, downloadErr := os.Open(urlP.RequestURI())
	if downloadErr != nil {
		klog.Error(downloadErr, " - urlP.RequestURI: ", urlP.RequestURI())
		return downloadErr
	}

	defer sourceFile.Close()

	// Create new file
	newFile, downloadErr := os.Create(chartZip)
	if downloadErr != nil {
		klog.Error(downloadErr, " - chartZip: ", chartZip)
		return downloadErr
	}

	defer newFile.Close()

	_, downloadErr = io.Copy(newFile, sourceFile)
	if downloadErr != nil {
		klog.Error(downloadErr)
		return downloadErr
	}

	return nil
}

func downloadFileHTTP(parentNamespace string, configMap *corev1.ConfigMap,
	fileURL string,
	secret *corev1.Secret,
	chartZip string) error {
	fileInfo, err := os.Stat(chartZip)
	if fileInfo != nil && fileInfo.IsDir() {
		downloadErr := fmt.Errorf("expecting chartZip to be a file but it's a directory: %s", chartZip)
		klog.Error(downloadErr)

		return downloadErr
	}

	if os.IsNotExist(err) {
		httpClient, downloadErr := GetHelmRepoClient(parentNamespace, configMap)
		if downloadErr != nil {
			klog.Error(downloadErr, " - Failed to create httpClient")
			return downloadErr
		}

		var req *http.Request

		req, downloadErr = http.NewRequest(http.MethodGet, fileURL, nil)
		if downloadErr != nil {
			klog.Error(downloadErr, "- Can not build request: ", "fileURL", fileURL)
			return downloadErr
		}

		if secret != nil && secret.Data != nil {
			req.SetBasicAuth(string(secret.Data["user"]), GetPassword(secret))
		}

		var resp *http.Response

		resp, downloadErr = httpClient.Do(req)
		if downloadErr != nil {
			klog.Error(downloadErr, "- Http request failed: ", "fileURL", fileURL)
			return downloadErr
		}

		if resp.StatusCode != 200 {
			downloadErr = fmt.Errorf("return code: %d unable to retrieve chart", resp.StatusCode)
			klog.Error(downloadErr, " - Unable to retrieve chart")

			return downloadErr
		}

		klog.V(5).Info("Download chart form helmrepo succeeded: ", fileURL)

		defer resp.Body.Close()

		var out *os.File

		out, downloadErr = os.Create(chartZip)
		if downloadErr != nil {
			klog.Error(downloadErr, " - Failed to create: ", chartZip)
			return downloadErr
		}

		defer out.Close()

		// Write the body to file
		_, downloadErr = io.Copy(out, resp.Body)
		if downloadErr != nil {
			klog.Error(downloadErr, " - Failed to copy body:", chartZip)
			return downloadErr
		}
	} else {
		klog.V(5).Info("Skip download chartZip already exists: ", chartZip)
	}

	return nil
}
