+++
title = "Provider Passworddepot"
linkTitle = "Provider Passworddepot"
weight = 1500
+++

External Secrets Operator integrates with [Password Depot API](https://www.password-depot.de/) to sync Password Depot to secrets held on the Kubernetes cluster.

### Authentication

The API requires a username and password. 


{{< readfile file=/snippets/password-depot-credentials-secret.yaml code="true" lang="yaml" >}}

### Update secret store
Be sure the `passworddepot` provider is listed in the `Kind=SecretStore` and host and database are set.

{{< readfile file=/snippets/passworddepot-secret-store.yaml code="true" lang="yaml" >}}


### Creating external secret

To sync a Password Depot variable to a secret on the Kubernetes cluster, a `Kind=ExternalSecret` is needed.

{{< readfile file=/snippets/passworddepot-external-secret.yaml code="true" lang="yaml" >}}

#### Using DataFrom

DataFrom can be used to get a variable as a JSON string and attempt to parse it.

{{< readfile file=/snippets/passworddepot-external-secret-json.yaml code="true" lang="yaml" >}}

### Getting the Kubernetes secret
The operator will fetch the project variable and inject it as a `Kind=Secret`.
```
kubectl get secret passworddepot-secret-to-create -o jsonpath='{.data.secretKey}' | base64 -d
```
