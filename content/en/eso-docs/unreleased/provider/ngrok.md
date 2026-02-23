+++
title = "ngrok"
linkTitle = "ngrok"
weight = 450
+++

## ngrok

External Secrets Operator integrates with [ngrok](https://ngrok.com/) to sync Kubernetes secrets with [ngrok Secrets for Traffic Policy](https://ngrok.com/blog-post/secrets-for-traffic-policy).
Currently, only pushing secrets is supported.

### Configuring ngrok Provider

Verify that `ngrok` provider is listed in the `Kind=SecretStore`. The properties `vault` and `auth` are required. The `apiURL` is optional and defaults to `https://api.ngrok.com`.


{{< readfile file=/snippets/ngrok-secret-store.yaml code="true" lang="yaml" >}}

### Pushing secrets to ngrok

To sync a Kubernetes secret with an external ngrok secret we need to create a PushSecret, this means a `Kind=PushSecret` is needed.

{{< readfile file=/snippets/ngrok-push-secret.yaml code="true" lang="yaml" >}}

#### PushSecret Metadata

Additionally, you can control the description and metadata of the secret in ngrok like so:

{{< readfile file=/snippets/ngrok-push-secret-with-metadata.yaml code="true" lang="yaml" >}}
