+++
title = "Fake"
linkTitle = "Fake"
weight = 220
+++

The Fake generator provides hard-coded key/value pairs. The intended use is just for debugging and testing.
The key/value pairs defined in `spec.data` is returned as-is.

## Example Manifest

{{< readfile file=/snippets/generator-fake.yaml code="true" lang="yaml" >}}

Example `ExternalSecret` that references the Fake generator:
{{< readfile file=/snippets/generator-fake-example.yaml code="true" lang="yaml" >}}
