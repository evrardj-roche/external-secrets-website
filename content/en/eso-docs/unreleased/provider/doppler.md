+++
title = "Doppler"
linkTitle = "Doppler"
weight = 300
+++

![Doppler External Secrets Provider](/img/doppler-provider-header.jpg)

## Doppler SecretOps Platform

Sync secrets from the [Doppler SecretOps Platform](https://www.doppler.com/) to Kubernetes using the External Secrets Operator.

## Authentication

Doppler supports two authentication methods:

> **NOTE:** When using a `ClusterSecretStore`, be sure to set `namespace` in `secretRef.dopplerToken` (for token auth) or `serviceAccountRef` (for OIDC auth).

### Service Token Authentication

Doppler [Service Tokens](https://docs.doppler.com/docs/service-tokens) are recommended as they restrict access to a single config.

![Doppler Service Token](/img/doppler-service-tokens.png)

> NOTE: Doppler Personal Tokens are also supported but require `project` and `config` to be set on the `SecretStore` or `ClusterSecretStore`.

Create the Doppler Token secret by opening the Doppler dashboard and navigating to the desired Project and Config, then create a new Service Token from the **Access** tab:

![Create Doppler Service Token](/img/doppler-create-service-token.jpg)

Create the Doppler Token Kubernetes secret with your Service Token value:

```sh
HISTIGNORE='*kubectl*' kubectl create secret generic \
    doppler-token-auth-api \
    --from-literal dopplerToken="dp.st.xxxx"
```

Then to create a generic `SecretStore`:

{{< readfile file=/snippets/doppler-generic-secret-store.yaml code="true" lang="yaml" >}}

### OIDC Authentication

For OIDC authentication, you'll need to configure a Doppler [Service Account Identity](https://docs.doppler.com/docs/service-account-identities) and create a Kubernetes ServiceAccount.

First, create a Kubernetes ServiceAccount:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: doppler-oidc-sa
  namespace: external-secrets
```

Next, create a Doppler Service Account Identity with:
- **Issuer**: Your cluster's OIDC discovery URL
- **Audience**: The resource-specific audience for the SecretStore (`secretStore:<namespace>:<storeName>` or `clusterSecretStore:<storeName>`), e.g. `secretStore:external-secrets:doppler-oidc-sa` or `clusterSecretStore:doppler-auth-api`
- **Subject**: The Kubernetes ServiceAccount (`system:serviceaccount:<serviceAccountNamespace>:<serviceAccountName>`), e.g. `system:serviceaccount:external-secrets:doppler-oidc-sa`

Then configure the SecretStore:

{{< readfile file=/snippets/doppler-oidc-secret-store.yaml code="true" lang="yaml" >}}


## Use Cases

The Doppler provider allows for a wide range of use cases:

1. [Fetch](#1-fetch)
2. [Fetch all](#2-fetch-all)
3. [Filter](#3-filter)
4. [JSON secret](#4-json-secret)
5. [Name transformer](#5-name-transformer)
6. [Download](#6-download)

Let's explore each use case using a fictional `auth-api` Doppler project.

## 1. Fetch

To sync one or more individual secrets:

{{< readfile file=/snippets/doppler-fetch-secret.yaml code="true" lang="yaml" >}}

![Doppler fetch](/img/doppler-fetch.png)

## 2. Fetch all

To sync every secret from a config:

{{< readfile file=/snippets/doppler-fetch-all-secrets.yaml code="true" lang="yaml" >}}

![Doppler fetch all](/img/doppler-fetch-all.png)

## 3. Filter

To filter secrets by `path` (path prefix), `name` (regular expression) or a combination of both:

{{< readfile file=/snippets/doppler-filtered-secrets.yaml code="true" lang="yaml" >}}

![Doppler filter](/img/doppler-filter.png)

## 4. JSON secret

To parse a JSON secret to its key-value pairs:

{{< readfile file=/snippets/doppler-parse-json-secret.yaml code="true" lang="yaml" >}}

![Doppler JSON Secret](/img/doppler-json.png)

## 5. Name transformer

Name transformers format keys from Doppler's UPPER_SNAKE_CASE to one of the following alternatives:

- upper-camel
- camel
- lower-snake
- tf-var
- dotnet-env
- lower-kebab

Name transformers require a specifically configured `SecretStore`:

{{< readfile file=/snippets/doppler-name-transformer-secret-store.yaml code="true" lang="yaml" >}}

Then an `ExternalSecret` referencing the `SecretStore`:

{{< readfile file=/snippets/doppler-name-transformer-external-secret.yaml code="true" lang="yaml" >}}

![Doppler name transformer](/img/doppler-name-transformer.png)

### 6. Download

A single `DOPPLER_SECRETS_FILE` key is set where the value is the secrets downloaded in one of the following formats:

- json
- dotnet-json
- env
- env-no-quotes
- yaml

Downloading secrets requires a specifically configured `SecretStore`:

{{< readfile file=/snippets/doppler-secrets-download-secret-store.yaml code="true" lang="yaml" >}}

Then an `ExternalSecret` referencing the `SecretStore`:

{{< readfile file=/snippets/doppler-secrets-download-external-secret.yaml code="true" lang="yaml" >}}

![Doppler download](/img/doppler-download.png)
