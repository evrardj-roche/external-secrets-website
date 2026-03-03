+++
title = "Doppler"
linkTitle = "Doppler"
weight = 270
+++

![Doppler External Secrets Provider](/doppler-provider-header.jpg)

## Doppler SecretOps Platform

Sync secrets from the [Doppler SecretOps Platform](https://www.doppler.com/) to Kubernetes using the External Secrets Operator.

## Authentication

Doppler [Service Tokens](https://docs.doppler.com/docs/service-tokens) are recommended as they restrict access to a single config.

![Doppler Service Token](/doppler-service-tokens.png)

> NOTE: Doppler Personal Tokens are also supported but require `project` and `config` to be set on the `SecretStore` or `ClusterSecretStore`.

Create the Doppler Token secret by opening the Doppler dashboard and navigating to the desired Project and Config, then create a new Service Token from the **Access** tab:

![Create Doppler Service Token](/doppler-create-service-token.jpg)

Create the Doppler Token Kubernetes secret with your Service Token value:

```sh
HISTIGNORE='*kubectl*' kubectl create secret generic \
    doppler-token-auth-api \
    --from-literal dopplerToken="dp.st.xxxx"
```

Then to create a generic `SecretStore`:

{{< readfile file=../snippets/doppler-generic-secret-store.yaml code="true" lang="yaml" >}}

> **NOTE:** In case of a `ClusterSecretStore`, be sure to set `namespace` in `secretRef.dopplerToken`.


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

{{< readfile file=../snippets/doppler-fetch-secret.yaml code="true" lang="yaml" >}}

![Doppler fetch](/doppler-fetch.png)

## 2. Fetch all

To sync every secret from a config:

{{< readfile file=../snippets/doppler-fetch-all-secrets.yaml code="true" lang="yaml" >}}

![Doppler fetch all](/doppler-fetch-all.png)

## 3. Filter

To filter secrets by `path` (path prefix), `name` (regular expression) or a combination of both:

{{< readfile file=../snippets/doppler-filtered-secrets.yaml code="true" lang="yaml" >}}

![Doppler filter](/doppler-filter.png)

## 4. JSON secret

To parse a JSON secret to its key-value pairs:

{{< readfile file=../snippets/doppler-parse-json-secret.yaml code="true" lang="yaml" >}}

![Doppler JSON Secret](/doppler-json.png)

## 5. Name transformer

Name transformers format keys from Doppler's UPPER_SNAKE_CASE to one of the following alternatives:

- upper-camel
- camel
- lower-snake
- tf-var
- dotnet-env
- lower-kebab

Name transformers require a specifically configured `SecretStore`:

{{< readfile file=../snippets/doppler-name-transformer-secret-store.yaml code="true" lang="yaml" >}}

Then an `ExternalSecret` referencing the `SecretStore`:

{{< readfile file=../snippets/doppler-name-transformer-external-secret.yaml code="true" lang="yaml" >}}

![Doppler name transformer](/doppler-name-transformer.png)

### 6. Download

A single `DOPPLER_SECRETS_FILE` key is set where the value is the secrets downloaded in one of the following formats:

- json
- dotnet-json
- env
- env-no-quotes
- yaml

Downloading secrets requires a specifically configured `SecretStore`:

{{< readfile file=../snippets/doppler-secrets-download-secret-store.yaml code="true" lang="yaml" >}}

Then an `ExternalSecret` referencing the `SecretStore`:

{{< readfile file=../snippets/doppler-secrets-download-external-secret.yaml code="true" lang="yaml" >}}

![Doppler download](/doppler-download.png)
