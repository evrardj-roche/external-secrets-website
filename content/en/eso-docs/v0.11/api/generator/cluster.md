+++
title = "Cluster Generator"
linkTitle = "Cluster Generator"
weight = 150
+++

`ClusterGenerator` is a generator wrapper that is available to configure a generator
cluster-wide. The purpose of this generator is that the user doesn't have to redefine
the generator in every namespace. They could define it once in the cluster and then reference that
in the consuming `ExternalSecret`.

## Limitations

With this, the generator will still create objects in the namespace in which the referencing ES lives.
That has not changed as of now. It will change in future modifications.

## Example Manifest

{{< readfile file=../../snippets/generator-cluster.yaml code="true" lang="yaml" >}}

Example `ExternalSecret` that references the Cluster generator:
{{< readfile file=../../snippets/generator-cluster-example.yaml code="true" lang="yaml" >}}
