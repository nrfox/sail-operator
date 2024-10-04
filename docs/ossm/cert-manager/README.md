[Return to OSSM Docs](../)

# About integrating Service Mesh with cert-manager and istio-csr

The cert-manager tool is a solution for X.509 certificate management on Kubernetes. It delivers a unified API to integrate applications with private or public key infrastructure (PKI), such as Vault, Google Cloud Certificate Authority Service, Let’s Encrypt, and other providers.

The cert-manager tool ensures the certificates are valid and up-to-date by attempting to renew certificates at a configured time before they expire.

For Istio users, cert-manager also provides integration with istio-csr, which is a certificate authority (CA) server that handles certificate signing requests (CSR) from Istio proxies. The server then delegates signing to cert-manager, which forwards CSRs to the configured CA server.

> **&#9432;** Red Hat provides support for integrating with istio-csr and cert-manager. Red Hat does not provide direct support for the istio-csr or the community cert-manager components. The use of community cert-manager shown here is for demonstration purposes only.

## Prerequisites

- One of these versions of cert-manager:
  - cert-manager Operator for Red Hat OpenShift 1.10 or later
  - community cert-manager Operator 1.11 or later
  - cert-manager 1.11 or later
- OpenShift Service Mesh Operator 3.0 or later
- istio-csr 0.6.0 or later

> **&#9432;** To avoid creating config maps in all namespaces when the istio-csr server is installed with the jetstack/cert-manager-istio-csr Helm chart, use the following setting: app.controller.configmapNamespaceSelector: "maistra.io/member-of: <istio-namespace>" in the istio-csr.yaml file.

## Installing cert-manager

You can install the cert-manager tool to manage the lifecycle of TLS certificates and ensure that they are valid and up-to-date. If you are running Istio in your environment, you can also install the istio-csr certificate authority (CA) server, which handles certificate signing requests (CSR) from Istio proxies. The istio-csr CA delegates signing to the cert-manager tool, which delegates to the configured CA.

### Procedure

1.  Create the `istio-system` namespace.

    ```sh
    oc create namespace istio-system
    ```

2.  Create the root cluster issuer:

    a. Create the cluster-issuer object as in the following example:
    Example `cluster-issuer.yaml`

        ```yaml
        apiVersion: cert-manager.io/v1
        kind: Issuer
        metadata:
          name: selfsigned
          namespace: istio-system
        spec:
          selfSigned: {}
        ---
        apiVersion: cert-manager.io/v1
        kind: Certificate
        metadata:
          name: istio-ca
          namespace: istio-system
        spec:
          isCA: true
          duration: 87600h # 10 years
          secretName: istio-ca
          commonName: istio-ca
          privateKey:
            algorithm: ECDSA
            size: 256
          subject:
            organizations:
            - cluster.local
            - cert-manager
          issuerRef:
            name: selfsigned
            kind: Issuer
            group: cert-manager.io
        ---
        apiVersion: cert-manager.io/v1
        kind: Issuer
        metadata:
          name: istio-ca
          namespace: istio-system
        spec:
          ca:
            secretName: istio-ca
        ```

    b. Create the object by using the following command:

        ```sh
        oc apply -f cluster-issuer.yaml
        ```

3.  Export the Root CA to the `cert-manager` namespace.

    ```sh
    oc get -n istio-system secret istio-ca -o jsonpath='{.data.tls\.crt}' | base64 -d > ca.pem
    oc create secret generic -n cert-manager istio-root-ca --from-file=ca.pem=ca.pem
    ```

4.  Install istio-csr:

    Next you will install istio-csr into the `cert-manager` namespace. Depending on which `updateStrategy` (`InPlace` or `RevisionBased`) you will choose for your `Istio` resource, you may need to pass additional options.

    > **&#9432;** Note: If your controlplane namespace is not `istio-system`, you will need to update `app.istio.namespace` to match your controlplane namespace.

    `InPlace` strategy installation

    ```sh
    helm repo add jetstack https://charts.jetstack.io --force-update
    helm upgrade cert-manager-istio-csr jetstack/cert-manager-istio-csr \
        --install \
        --namespace cert-manager \
        --wait \
        --set "app.tls.rootCAFile=/var/run/secrets/istio-csr/ca.pem" \
        --set "volumeMounts[0].name=root-ca" \
        --set "volumeMounts[0].mountPath=/var/run/secrets/istio-csr" \
        --set "volumes[0].name=root-ca" \
        --set "volumes[0].secret.secretName=istio-root-ca" \
        --set "app.istio.namespace=istio-system"
    ```

    `RevisionBased` strategy installation

    For the `RevisionBased` strategy, you need to specify all the istio revisions to your [istio-csr deployment](https://github.com/cert-manager/istio-csr/tree/main/deploy/charts/istio-csr#appistiorevisions0--string). The name for this will match this scheme: `<.metadata.name-.spec.version>` e.g. for an `Istio` resource named `default` with a `spec.version` of `1.23.0`, this would be `default-v1-23-0`.

    ```sh
    helm repo add jetstack https://charts.jetstack.io --force-update
    helm upgrade cert-manager-istio-csr jetstack/cert-manager-istio-csr \
        --install \
        --namespace cert-manager \
        --wait \
        --set "app.tls.rootCAFile=/var/run/secrets/istio-csr/ca.pem" \
        --set "volumeMounts[0].name=root-ca" \
        --set "volumeMounts[0].mountPath=/var/run/secrets/istio-csr" \
        --set "volumes[0].name=root-ca" \
        --set "volumes[0].secret.secretName=istio-root-ca" \
        --set "app.istio.namespace=istio-system"
        --set "app.istio.revisions={default-v1-23-0}"
    ```

5.  Install your `Istio` resource

    Update your `updateStrategy` accordingly.

    ```sh
    oc apply -f - <<EOF
    apiVersion: sailoperator.io/v1alpha1
    kind: Istio
    metadata:
      name: default
    spec:
      version: v1.23.0
      namespace: istio-system
      updateStrategy:
        type: RevisionBased
      values:
        global:
          caAddress: cert-manager-istio-csr.cert-manager.svc:443
        pilot:
          env:
            ENABLE_CA_SERVER: "false"
          volumeMounts:
            - mountPath: /tmp/var/run/secrets/istiod/tls
              name: istio-csr-dns-cert
              readOnly: true
    EOF
    ```

Verification

Use the sample httpbin service and sleep app to check mTLS traffic from ingress gateways and verify that the cert-manager tool is installed.

    Deploy the HTTP and sleep apps:

    $ oc new-project <namespace>

    $ oc apply -f https://raw.githubusercontent.com/maistra/istio/maistra-2.4/samples/httpbin/httpbin.yaml

    $ oc apply -f https://raw.githubusercontent.com/maistra/istio/maistra-2.4/samples/sleep/sleep.yaml

    Verify that sleep can access the httpbin service:

    $ oc exec "$(oc get pod -l app=sleep -n <namespace> \
       -o jsonpath={.items..metadata.name})" -c sleep -n <namespace> -- \
       curl http://httpbin.<namespace>:8000/ip -s -o /dev/null \
       -w "%{http_code}\n"

    Example output

    200

    Check mTLS traffic from the ingress gateway to the httpbin service:

    $ oc apply -n <namespace> -f https://raw.githubusercontent.com/maistra/istio/maistra-2.4/samples/httpbin/httpbin-gateway.yaml

    Get the istio-ingressgateway route:

    INGRESS_HOST=$(oc -n istio-system get routes istio-ingressgateway -o jsonpath='{.spec.host}')

    Verify mTLS traffic from the ingress gateway to the httpbin service:

    $ curl -s -I http://$INGRESS_HOST/headers -o /dev/null -w "%{http_code}" -s

Additional resources

For information about how to install the cert-manager Operator for OpenShift Container Platform, see: Installing the cert-manager Operator for Red Hat OpenShift.

Below are instructions for integrating cert-manager with OpenShift Service Mesh 3. It largely follows the [cert-manager istio-csr documentation](https://cert-manager.io/docs/usage/istio-csr/) but you will need to adjust a few settings based on which `updateStrategy` you are using.

3. [Verify your deployment](#verify) is configured correctly.

## RevisionBased Strategy

1. Install [istio-csr](https://cert-manager.io/docs/usage/istio-csr).

   ```sh
   helm repo add jetstack https://charts.jetstack.io --force-update

   helm upgrade cert-manager-istio-csr jetstack/cert-manager-istio-csr \
     --install \
     --namespace cert-manager \
     --wait \
     --set "app.tls.rootCAFile=/var/run/secrets/istio-csr/ca.pem" \
     --set "volumeMounts[0].name=root-ca" \
     --set "volumeMounts[0].mountPath=/var/run/secrets/istio-csr" \
     --set "volumes[0].name=root-ca" \
     --set "volumes[0].secret.secretName=istio-root-ca" \
     --set "app.istio.namespace=istio-system" \
     --set "app.istio.revisions={default-v1-23-0}"
   ```

   Note: If your controlplane namespace is not `istio-system`, you will need to update `app.istio.namespace` to match your controlplane namespace.

2. Install `Istio` controlplane in the `istio-system` namespace.

3. [Verify your deployment](#verify) is configured correctly.

### Verify

1. Deploy a sample application.

   Create the `sample` namespace.

   ```sh
   oc create namespace sample
   ```

   For the `RevisionBased` strategy, label your namespace with `istio.io/rev=default-v1-23-0`.

   ```sh
   oc label namespace sample istio.io/rev=default-v1-23-0
   ```

   For the `InPlace` strategy, label your namespace with `istio.io/rev=default`.

   ```sh
   oc label namespace sample istio.io/rev=default
   ```

   Deploy the `httpbin` application.

   ```sh
   oc apply -n sample -f https://raw.githubusercontent.com/istio/istio/release-1.23/samples/httpbin/httpbin.yaml
   ```

2. Ensure httpbin pod is Running.

   ```sh
   oc get pods -n sample
   ```

   ```sh
   NAME                       READY   STATUS    RESTARTS   AGE
   httpbin-67854dd9b5-b7c2q   2/2     Running   0          110s
   ```

3. Use `istioctl` to ensure httpbin workload certificate matches what is expected.

   ```sh
   istioctl proxy-config secret -n sample $(oc get pods -n sample -o jsonpath='{.items..metadata.name}' --selector app=httpbin) -o json | jq -r '.dynamicActiveSecrets[0].secret.tlsCertificate.certificateChain.inlineBytes' | base64 --decode | openssl x509 -text -noout
   ```

### Upgrades - RevisionBased

Because istio-csr requires you to pass all revisions, each time you upgrade your `RevsionBased` controlplane you will need to **first** update your istio-csr deployment with the new revision before you update your `Istio.spec.version`.

```sh
helm upgrade cert-manager-istio-csr jetstack/cert-manager-istio-csr \
  --install \
  --namespace cert-manager \
  --wait \
  --reuse-values \
  --set "app.istio.revisions={default-v1-23-0,default-v1-23-1}"
```

Once the old revision is no longer in use, you can remove the revision from istio-csr as well.

```sh
helm upgrade cert-manager-istio-csr jetstack/cert-manager-istio-csr \
  --install \
  --namespace cert-manager \
  --wait \
  --reuse-values \
  --set "app.istio.revisions={default-v1-23-1}"
```
