# Deployment Guide

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Deployment Guide](#deployment-guide)
    - [Environment variable](#environment-variable)
    - [RBAC](#rbac)
        - [Deployment](#deployment)
    - [General process with Subscriptions](#general-process-with-subscriptions)
        - [HelmChartSubscriptions](#helmchartsubscriptions)
        - [Helm-charts filtering](#helm-charts-filtering)
        - [Authentication](#authentication)
        - [Helm-repo client configuration](#helm-repo-client-configuration)
    - [General process without Subscriptions](#general-process-without-subscriptions)
<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Environment variable

The environment variable `CHARTS_DIR` must be set when developing, it specifies the directory where the charts will be downloaded and expanded (Default `/tmp/charts`).

## RBAC

The service account is `multicloud-operators-subscription-release`.

The cluster-role `cluster-admin` is binded to the service account as the helmrelease operator must be able to deploy helm chart in any namespace.

The role `multicloud-operators-subscription-release` is binded to that service account.

A cluster role `multicloud-operators-subscription-release` is created for the resources `helmchartsubscription` and `helmrelease`.

In order for another service account to be able to access the `helmchartsubscription` and `helmrelease`, a role binding must be create.

### Deployment

1) Do `kubectl apply -f` on all files in deploy/crds.*-crd.yaml
2) `kubectl apply -f service_account.yaml`
3) `kubectl apply -f role.yaml`
4) `kubectl apply -f role_binding.yaml`
5) `kubectl apply -f operator.yaml`

## General process with Subscriptions

The operator generates `HelmRelease` CR for each chart to deploy based on the subscription in the same namespace and named `<s.Name>-<chart_name>[-<channel_name>]`. The channel_name is added only if the channel attribute is set in the subscription.

To do so the helmchartsubscription controller follows these steps:

1) Read the index.yaml at the source address.
2) Filter the index.yaml with the spec.Name and spec.packageFilter of the helmchartsubscription CR.
3) Take the last version of a chart if multiple version are still present for the same chart after filtering.
4) Create a HelmRelease for each entries in the filtered index.yaml

### HelmChartSubscriptions

The subscription operator watches `HelmChartSubscription` and `HelmRelease` CRs.

The User creates a HelmChartSubscription CR. if installPlanApproval is set to `Automatic` then the helmrepo will be monitored and new chart version will be deployed, if set to `Manual` then no automatic deployment.

```yaml
apiVersion: app.ibm.com/v1alpha1
kind: HelmChartSubscription
metadata:
  name: myapp
  namespace: default
spec:
  channel: default/ope
  installPlanApproval: Automatic
  secretRef:
    name: mysecret
  configRef:
    name: mycluster-config
  name: ibm-myapp-api #Optional but be careful to write your filters to refine the number of chart to be deployed.
  packageFilter:
    labelSelector:
      matchLabels:
        "MCM": "true"
    annotations:
      tillerVersion: 2.4.0
    version: '>0.2.2'
  packageOverrides:
  - packageName: ibm-myapp-api
    packageOverrides:
    - path: spec.values
      value: |
        attribute1: value1
        attribute2: value2
  chartsSource:
    helmrepo:
      urls:
      - https://mycluster.icp:8443/helm-repo/charts
  ```

  Source can have the following format for github (not yet fully implemented):

  ``` yaml
  chartsSource:
    type: github
    github:
      urls:
      - https://github.ibm.com/IBMPrivateCloud/hybrid-cluster-manager-v2-chart.git
      chartsPath: 3.2.1-examples/guestbook-kube-subscription
      branch: master
  ```

branch master is the default.

### Helm-charts filtering

The optional spec.name defines the name of the helm-chart, it can be also a regex if multiple helm-charts must be deployed.

The optional spec.packageFilter allows to filter the helm-charts.
Filtering is done on:

- the version of the helm-chart (semver expression),
- the tiller version of the helm-chart (Should may be removed as the operator has its own tiller)
- the digest must match
- the labelSelector allows to check if the helm-charts has the required keywords.

### Authentication

A secretRef can be provided in the subscriptionRelease spec. It references a secret where the authentication parameter to access the helm-repo are set.
The attributes are either `user` and `password` or `authHeader`. All values must be base64 encoded.
The `authHeader` format is `<Auth_type> <token>` and so for example:
`Bearer xxxxxx`.

### Helm-repo client configuration

The configRef is a reference to a configMap which holds the parameters to the helm-repo.

```yaml
apiVersion: v1
data:
  insecureSkipVerify: "true"
kind: ConfigMap
metadata:
  name: mycluster-config
  namespace: default
```

The HelmReleases are owned by the HelmChartSubscription and so if the subscription is deleted the release is deleted too.

```yaml
apiVersion: app.ibm.com/v1alpha1
kind: HelmRelease
metadata:
  annotations:
    app.ibm.com/hosting-deployable: default/ope
    app.ibm.com/hosting-subscription: default/myapp
  creationTimestamp: 2019-08-12T09:01:52Z
  generation: 1
  name: myapp-ibm-myapp-api-ope
  namespace: default
  ownerReferences:
  - apiVersion: app.ibm.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: HelmChartSubscription
    name: myapp
    uid: ec3c8f28-bcde-11e9-b55f-fa163e0cb658
  resourceVersion: "3852059"
  selfLink: /apis/app.ibm.com/v1alpha1/namespaces/default/subscriptionreleases/myapp-ibm-myapp-api-ope
  uid: d35adca8-bcdf-11e9-b55f-fa163e0cb658
spec:
  source:
    type: helmrepo
    helmRepo:
      URLs:
      - https://mycluster.icp:8443/helm-repo/requiredAssets/ibm-myapp-api-0.2.3-015-20190725140717.tgz
  chartName: ibm-myapp-api
  secretRef:
    name: mysecret
  configRef:
    name: mycluster-config
    value: |
      attribute1: value1
      attribute2: value2
  version: 0.2.3-015-20190725140717
```

Source can have the following format for github:

```yaml
  source:
    github:
      urls:
      - https://github.ibm.com/IBMPrivateCloud/icp-cert-manager-chart
      chartPath: stable/ibm-cert-manager
      branch: master
    type: github
```

Branch master is the default.

Once the HelmRelease is created or modified, the operator will deploy each charts specified in each HelmRelease.

To do so, the following steps are taken:

1) Download the chart tgz in the `$CHARTS_DIR`.
2) Unzip the tgz in `$CHARTS_DIR/<sr.Spec.ReleaseName>/<sr.namespace>/<chart_name>`
3) Create a manager with the values provided in the HelmRelease
4) Launch the deployment.

## General process without Subscriptions

The operator can run with the helmchartsubscription controller disabled by adding the flag `--helmchart-subscription-controller-disabled` to the operator launch command. Helmrelease CR are then created manually.

```yaml
apiVersion: app.ibm.com/v1alpha1
kind: HelmRelease
metadata:
  name: myapp-ibm-myapp-api-ope
  namespace: default
spec:
  source:
    type: helmrepo
    helmRepo:
      URLs:
      - https://mycluster.icp:8443/helm-repo/requiredAssets/ibm-myapp-api-0.2.3-015-20190725140717.tgz
  chartName: ibm-myapp-api
  secretRef:
    name: mysecret
  configRef:
    name: mycluster-config
    value: |
      attribute1: value1
      attribute2: value2
```

`file:` sheme is also supported to define the location of a local file.

Source can have the following format for github:

```yaml
  source:
    github:
      urls:
      - https://github.ibm.com/IBMPrivateCloud/icp-cert-manager-chart
      chartPath: stable/ibm-cert-manager
      branch: master
    type: github
```
