+++
title = "ClusterPushSecret"
linkTitle = "ClusterPushSecret"
weight = 90
+++

The `ClusterPushSecret` is a cluster scoped resource that can be used to manage `PushSecret` resources in specific namespaces.

With `namespaceSelectors` you can select namespaces in which the PushSecret should be created.
If there is a conflict with an existing resource the controller will error out.

## Example

Below is an example of the `ClusterPushSecret` in use.

{{< readfile file=/snippets/full-cluster-push-secret.yaml code="true" lang="yaml" >}}

The result of the created Secret object will look like:

```yaml
# The destination secret that will be templated and pushed by ClusterPushSecret.
apiVersion: v1
kind: Secret
metadata:
  name: destination-secret
stringData:
  best-pokemon-dst: "PIKACHU is the really best!"
```
