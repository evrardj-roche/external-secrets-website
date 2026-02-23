+++
title = "Kubernetes Secret Types"
linkTitle = "Kubernetes Secret Types"
weight = 100
+++

# A few common k8s secret types examples

Here we will give some examples of how to work with a few common k8s secret types. We will give this examples here with the gcp provider (should work with other providers in the same way). Please also check the guides on [Advanced Templating](templating.md) to understand the details.

Please follow the authentication and SecretStore steps of the [Google Cloud Secrets Manager guide](../provider/google-secrets-manager.md) to setup access to your google cloud account first.


## Dockerconfigjson example

First create a secret in Google Cloud Secrets Manager containing your docker config:

![iam](/img/screenshot_docker_config_json_example.png)

Let's call this secret docker-config-example on Google Cloud.

Then create a ExternalSecret resource taking advantage of templating to populate the generated secret:

{{< readfile file=/snippets/gcpsm-docker-config-externalsecret.yaml code="true" lang="yaml" >}}

For Helm users: since Helm interprets the template above, the ExternalSecret resource can be written this way:

{{< readfile file=/snippets/gcpsm-docker-config-helm-externalsecret.yaml code="true" lang="yaml" >}}

For more information, please see [this issue](https://github.com/helm/helm/issues/2798)

This will generate a valid dockerconfigjson secret for you to use!

You can get the final value with:

```bash
kubectl get secret secret-to-be-created -n <namespace> -o jsonpath="{.data.\.dockerconfigjson}" | base64 -d
```

Alternately, if you only have the container registry name and password value, you can take advantage of the advanced ExternalSecret templating functions to create the secret:

```yaml

apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: dk-cfg-example
spec:
  refreshInterval: 1h0m0s
  secretStoreRef:
    name: example
    kind: SecretStore
  target:
    template:
      type: kubernetes.io/dockerconfigjson
      data:
        .dockerconfigjson: '{"auths":{"{{ .registryName | lower }}.{{ .registryHost }}":{"username":"{{ .registryName }}","password":"{{ .password }}","auth":"{{ printf "%s:%s" .registryName .password | b64enc }}"}}}'
  data:
  - secretKey: registryName
    remoteRef:
      key: secret/docker-registry-name # "myRegistry"
  - secretKey: registryHost
    remoteRef:
      key: secret/docker-registry-host # "docker.io"
  - secretKey: password
    remoteRef:
      key: secret/docker-registry-password

```

## TLS Cert example

We are assuming here that you already have valid certificates, maybe generated with letsencrypt or any other CA. So to simplify you can use openssl to generate a single secret pkcs12 cert based on your cert.pem and privkey.pen files.

```bash
openssl pkcs12 -export -out certificate.p12 -inkey privkey.pem -in cert.pem
```

With a certificate.p12 you can upload it to Google Cloud Secrets Manager:

![p12](/img/screenshot_ssl_certificate_p12_example.png)

And now you can create an ExternalSecret that gets it. You will end up with a k8s secret of type tls with pem values.

{{< readfile file=/snippets/gcpsm-tls-externalsecret.yaml code="true" lang="yaml" >}}

You can get their values with:

```bash
kubectl get secret secret-to-be-created -n <namespace> -o jsonpath="{.data.tls\.crt}" | base64 -d
kubectl get secret secret-to-be-created -n <namespace> -o jsonpath="{.data.tls\.key}" | base64 -d
```


## SSH Auth example

Add the ssh privkey to a new Google Cloud Secrets Manager secret:

![ssh](/img/screenshot_ssh_privkey_example.png)

And now you can create an ExternalSecret that gets it. You will end up with a k8s secret of type ssh-auth with the privatekey value.

{{< readfile file=/snippets/gcpsm-ssh-auth-externalsecret.yaml code="true" lang="yaml" >}}

You can get the privkey value with:

```bash
kubectl get secret secret-to-be-created -n <namespace> -o jsonpath="{.data.ssh-privatekey}" | base64 -d
```

## More examples

!!! note "We need more examples here"
    Feel free to contribute with our docs and add more examples here!
