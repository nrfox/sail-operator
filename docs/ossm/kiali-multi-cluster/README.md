[Return to OSSM Docs](../)

# Kiali - multi-cluster

For multi-cluster Istio deployments, Kiali can show you a single unified view of your mesh across clusters.

Before proceeding with the setup, ensure you meet the requirements.

## Requirements

- Two openshift clusters.
- Istio installed in a multi-cluster configuration on each cluster.
- IstioCNI installed on each cluster.
- Aggregated metrics and traces. Kiali needs a single endpoint for metrics and a single endpoint for traces where it can consume aggregated metrics/traces across all clusters. There are multiple ways to aggregate metrics/traces such as Prometheus federation or using OTEL collector pipelines.
- Cluster Admin for each cluster.
- Kiali Operator v1.89 installed on the `east` cluster.

## Setup

In this tutorial, we will deploy Kiali on the `east` cluster and then grant Kiali access to the `west` cluster. The unified multi-cluster setup requires the Kiali Service Account (SA) to have read access to each Kubernetes cluster in the mesh. This is separate from the user credentials that are required when a user logs into Kiali. Kiali uses the user's credentials to check if the user has access to a namespace and when performing any write operation such as creating/editing/deleting objects in Kubernetes. To give the Kiali Service Account access to each remote cluster, a kubeconfig with credentials needs to be created and mounted into the Kiali pod.

If you would like to keep a separate Kiali per cluster and do not want to give Kiali access to remote clusters, you can still manually specify the remote cluster and remote Kiali URLs in the Kiali configuration and the UI will try to provide links to the external Kiali where appropriate. See below for more details.

### Procedure

1. Install Kiali on the `east` cluster.

   Create a file named `kiali.yaml`.

   ```yaml
   apiVersion: kiali.io/v1alpha1
   kind: Kiali
   metadata:
     name: kiali
     namespace: istio-system
   spec:
     version: v1.89
   ```

   Apply the yaml file into the `east` cluster.

   ```sh
   oc --context east apply -f kiali.yaml
   ```

   Wait for Kiali to become ready.

   ```sh
   oc --context east wait --for=condition=Successful --timeout=60s kialis/kiali -n istio-system
   ```

1. Create an `OAuthClient` on the remote cluster so that Kiali can access the OpenShift API server on behalf of users.

   Find your Kiali URL

   ```sh
   oc --context east get route kiali -n istio-system -o jsonpath='{.spec.host}'
   ```

   Create a file named `oauthclientwest.yaml`

   ```yaml
   apiVersion: oauth.openshift.io/v1
   grantMethod: auto
   kind: OAuthClient
   metadata:
     labels:
       app: kiali
       app.kubernetes.io/instance: kiali
       app.kubernetes.io/name: kiali
       app.kubernetes.io/part-of: kiali
     name: kiali-istio-system
   redirectURIs:
     - https://<your-kiali-route>/api/auth/callback/west
   ```

   Create the `OAuthClient` in the west cluster.

   ```sh
   oc --context west apply -f oauthclientwest.yaml
   ```

1. Create a remote cluster secret.

   In order to access a remote cluster, you must provide a kubeconfig to Kiali via a Kubernetes secret. You can use [this script](https://raw.githubusercontent.com/kiali/kiali/master/hack/istio/multicluster/kiali-prepare-remote-cluster.sh) to simplify this process for you. Running this script will:

   - Create a Service Account for Kiali in the remote cluster.
   - Create RBAC resources for this Service Account in the remote cluster.
   - Create a kubeconfig file and save this as a secret in the namespace where Kiali is deployed on the `east` cluster.

   1. Download the `kiali-prepare-remote-cluster` script.

      ```sh
      curl -L -o kiali-prepare-remote-cluster.sh https://raw.githubusercontent.com/kiali/kiali/master/hack/istio/multicluster/kiali-prepare-remote-cluster.sh
      ```

   2. Make the script executeable.

      ```sh
      chmod +x kiali-prepare-remote-cluster.sh
      ```

   3. Run the script passing your `east` and `west` cluster contexts.

      ```sh
      ./kiali-prepare-remote-cluster.sh --kiali-cluster-context east --remote-cluster-context west --view-only false --kiali-resource-name kiali --remote-cluster-namespace istio-system --remote-cluster-name west
      ```

   **Note:** Use the option `--help` for additional details on how to use the script.

1. Restart Kiali to pickup the remote secret.

   ```sh
   oc --context east rollout restart deployments/kiali -n istio-system
   ```

   Wait for Kiali to become ready

   ```sh
   oc --context east rollout status deployments/kiali -n istio-system
   ```

1. Login to Kiali.

   When you first visit Kiali, you will login to the cluster where Kiali is deployed. In our case it will be the `east` cluster.

   ```sh
   oc --context east get route -l app=kiali -n istio-system
   ```

1. Login to the `west` cluster through Kiali.

   In order to see other clusters in the Kiali UI, you must first login as a user to those clusters through Kiali. Click on the user profile dropdown in the top right hand menu. Then select `Login to west`. You will again be redirected to an openshift login page and prompted for credentials but this will be for the `west` cluster.

1. Verify that you see namespaces from each cluster on the overview page.

## Disconnected Kiali deployment

Kiali can also be deployed without access to remote clusters. In this setup, you will deploy a Kiali to each cluster and then configure each Kiali to link to each other. Kiali won't be able to show you a unified view of your mesh but you can easily jump from one Kiali to the other.

### Requirements

- Two openshift clusters.
- Istio installed in a multi-cluster configuration on each cluster.
- IstioCNI installed on each cluster.
- Aggregated metrics and traces. Kiali needs a single endpoint for metrics and a single endpoint for traces where it can consume aggregated metrics/traces across all clusters. There are multiple ways to aggregate metrics/traces such as Prometheus federation or using OTEL collector pipelines.
- Cluster Admin for each cluster.
- Kiali Operator installed on each cluster.

1. Install Kiali on the `east` cluster.

   Create a file named `kiali-east.yaml`.

   ```yaml
   apiVersion: kiali.io/v1alpha1
   kind: Kiali
   metadata:
     name: kiali
     namespace: istio-system
   spec:
     version: v1.89
   ```

   Apply the yaml file into the `east` cluster.

   ```sh
   oc --context east apply -f kiali-east.yaml
   ```

   Wait for Kiali to become ready.

   ```sh
   oc --context east wait --for=condition=Successful --timeout=60s kialis/kiali -n istio-system
   ```

1. Install Kiali on the `west` cluster.

   Create a file named `kiali-west.yaml`.

   ```yaml
   apiVersion: kiali.io/v1alpha1
   kind: Kiali
   metadata:
     name: kiali
     namespace: istio-system
   spec:
     version: v1.89
   ```

   Apply the yaml file into the `west` cluster.

   ```sh
   oc --context west apply -f kiali-west.yaml
   ```

   Wait for Kiali to become ready.

   ```sh
   oc --context west wait --for=condition=Successful --timeout=60s kialis/kiali -n istio-system
   ```

1. Modify the `east` cluster Kiali to link to the `west` cluster Kiali.

   Find the URL for the `west` cluster Kiali.

   ```sh
   oc --context west get route kiali -n istio-system -o jsonpath='{.spec.host}'
   ```

   Modify `kiali-east.yaml`.

   ```yaml
   apiVersion: kiali.io/v1alpha1
   kind: Kiali
   metadata:
     name: kiali
     namespace: istio-system
   spec:
     version: v1.89
     clustering:
       clusters:
         - name: west
       kiali_urls:
         - cluster_name: west
           instance_name: kiali
           namespace: istio-system
           url: https://<kiali-url>
   ```

   Apply the yaml file into the `east` cluster.

   ```sh
   oc --context east apply -f kiali-east.yaml
   ```

   Wait for Kiali to become ready.

   ```sh
   oc --context east wait --for=condition=Successful --timeout=60s kialis/kiali -n istio-system
   ```

1. Modify the `west` cluster Kiali to link to the `east` cluster Kiali.

   Find the URL for the `east` cluster Kiali.

   ```sh
   oc --context east get route kiali -n istio-system -o jsonpath='{.spec.host}'
   ```

   Modify `kiali-west.yaml`.

   ```yaml
   apiVersion: kiali.io/v1alpha1
   kind: Kiali
   metadata:
     name: kiali
     namespace: istio-system
   spec:
     version: v1.89
     clustering:
       clusters:
         - name: east
       kiali_urls:
         - cluster_name: east
           instance_name: kiali
           namespace: istio-system
           url: https://<kiali-url>
   ```

   Apply the yaml file into the `west` cluster.

   ```sh
   oc --context west apply -f kiali-west.yaml
   ```

   Wait for Kiali to become ready.

   ```sh
   oc --context west wait --for=condition=Successful --timeout=60s kialis/kiali -n istio-system
   ```

1. Verify you can see each cluster on the mesh page.

   Open Kiali in the `east` cluster.

   ```sh
   oc --context east get route kiali -n istio-system -o jsonpath='{.spec.host}'
   ```

   Navigate to the `Mesh` page from the lefthand menu and verify that you see Kiali in the `west` cluster.

   Open Kiali in the `west` cluster.

   ```sh
   oc --context west get route kiali -n istio-system -o jsonpath='{.spec.host}'
   ```

   Navigate to the `Mesh` page from the lefthand menu and verify that you see Kiali in the `east` cluster.
