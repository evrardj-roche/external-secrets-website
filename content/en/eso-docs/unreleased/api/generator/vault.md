+++
title = "Vault Dynamic Secret"
linkTitle = "Vault Dynamic Secret"
weight = 200
+++

The `VaultDynamicSecret` Generator provides an interface to HashiCorp Vault's
[Secrets engines](https://developer.hashicorp.com/vault/docs/secrets). Specifically,
it enables obtaining dynamic secrets not covered by the
[HashiCorp Vault provider](../../provider/hashicorp-vault.md).

Any Vault authentication method supported by the provider can be used here
(`provider` block of the spec).

All secrets engines should be supported by providing matching `path`, `method`
and `parameters` values to the Generator spec (see example below).

Exact output keys and values depend on the Vault secret engine used; nested values
are stored into the resulting Secret in JSON format. The generator exposes `data`
section of the response from Vault API by default. To adjust the behaviour, use
`resultType` key.

## Example manifest

{{< readfile file=/snippets/generator-vault.yaml code="true" lang="yaml" >}}

Example `ExternalSecret` that references the Vault generator:
{{< readfile file=/snippets/generator-vault-example.yaml code="true" lang="yaml" >}}
