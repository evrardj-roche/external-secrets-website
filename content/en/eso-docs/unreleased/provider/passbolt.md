+++
title = "Passbolt"
linkTitle = "Passbolt"
weight = 360
+++

External Secrets Operator integrates with [Passbolt API](https://www.passbolt.com/) to sync Passbolt to secrets held on the Kubernetes cluster.



### Creating a Passbolt secret store

Be sure the `passbolt` provider is listed in the `Kind=SecretStore` and auth and host are set.
The API requires a password and private key provided in a secret.

{{< readfile file=/snippets/passbolt-secret-store.yaml code="true" lang="yaml" >}}


### Creating an external secret

To sync a Passbolt secret to a Kubernetes secret, a `Kind=ExternalSecret` is needed.
By default the secret contains name, username, uri, password and description.

To only select a single property add the `property` key.

{{< readfile file=/snippets/passbolt-external-secret-example.yaml code="true" lang="yaml" >}}

The above external secret will lead to the creation of a secret in the following form:

{{< readfile file=/snippets/passbolt-secret-example.yaml code="true" lang="yaml" >}}


### Finding a secret by name

Instead of retrieving secrets by ID you can also use `dataFrom` to search for secrets by name.

{{< readfile file=/snippets/passbolt-external-secret-findbyname.yaml code="true" lang="yaml" >}}
