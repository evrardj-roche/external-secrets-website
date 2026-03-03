+++
title = "ClusterExternalSecret"
linkTitle = "ClusterExternalSecret"
weight = 80
+++

![ClusterExternalSecret](/diagrams-cluster-external-secrets.png)

The `ClusterExternalSecret` is a cluster scoped resource that can be used to manage `ExternalSecret` resources in specific namespaces.

With `namespaceSelectors` you can select namespaces in which the ExternalSecret should be created.
If there is a conflict with an existing resource the controller will error out.

## Example

Below is an example of the `ClusterExternalSecret` in use.

{{< readfile file=../snippets/full-cluster-external-secret.yaml code="true" lang="yaml" >}}

## Deprecations

### namespaceSelector

The field `namespaceSelector` has been deprecated in favor of `namespaceSelectors` and will be removed in a future
version.
