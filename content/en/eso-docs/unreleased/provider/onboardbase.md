+++
title = "Onboardbase"
linkTitle = "Onboardbase"
weight = 380
+++


![Onboardbase External Secrets Provider](/img/onboardbase-provider.png)

## Onboardbase Secret Management

Sync secrets from [Onboardbase](https://www.onboardbase.com/) to Kubernetes using the External Secrets Operator.

## Authentication

### Get an Onboardbase [API Key](https://docs.onboardbase.com/reference/api-auth).

Create the Onboardbase API by opening the organization tab under your account settings:

![Onboardabse API Key](/img/onboardbase-api-key.png)



And view them under the team name in your Account settings


![Onboardabse API Key](/img/onboardbase-create-api-key.png)

Create an Onboardbase API secret with your API Key and Passcode value:

```sh
HISTIGNORE='*kubectl*' \
  kubectl create secret generic onboardbase-auth-secret \
  --from-literal=API_KEY=*****VZYKYJNMMEMK***** \
  --from-literal=PASSCODE=api-key-passcode
```

Then to create a generic `SecretStore`:

{{< readfile file=/snippets/onboardbase-generic-secret-store.yaml code="true" lang="yaml" >}}

## Use Cases

The below operations are possible with the Onboardbase provider:

1. [Fetch](#1-fetch)
2. [Fetch all](#2-fetch-all)
3. [Filter](#3-filter)

Let's explore each use case using a fictional `auth-api` Onboardbase project.

### 1. Fetch

To sync one or more individual secrets:

{{< readfile file=/snippets/onboardbase-fetch-secret.yaml code="true" lang="yaml" >}}

### 2. Fetch all

To sync every secret from a config:

{{< readfile file=/snippets/onboardbase-fetch-all-secrets.yaml code="true" lang="yaml" >}}

### 3. Filter

To filter secrets by `path` (path prefix), `name` (regular expression) or a combination of both:

{{< readfile file=/snippets/onboardbase-filtered-secrets.yaml code="true" lang="yaml" >}}
