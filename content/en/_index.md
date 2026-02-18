+++
title = "External Secrets"
description = "Securely Sync secrets between external systems and Kubernetes"
+++

{{% blocks/cover title="Welcome to External-Secrets" image_anchor="top" height="min td-below-navbar" %}}

<!-- prettier-ignore -->
{{% param description %}}
{.display-6}

<!-- prettier-ignore -->
<div class="td-cta-buttons my-5">
  <a {{% _param btn-lg primary %}} href="eso-docs/">
    Read the Operator docs
  </a>
  <a {{% _param btn-lg secondary %}}
    href="{{% param github_project_repo %}}"
    target="_blank" rel="noopener noreferrer">
    Get the Operator code
    {{% _param FA brands github "" %}}
  </a>
<!--
  <a {{% _param btn-lg primary %}} href="reloader-docs/">
    Read the reloader docs
  </a>
  <a {{% _param btn-lg secondary %}}
    href="{{% param github_reloader_repo %}}"
    target="_blank" rel="noopener noreferrer">
    Get the Reloader code
    {{% _param FA brands github "" %}}
  </a>
-->
</div>


<!-- continue below! -->

{{% blocks/link-down color="info" %}}

{{% /blocks/cover %}}

{{% blocks/lead color="white" %}}
The External-Secrets is a series of tools revolving around secret management in kubernetes.
Our aim is to make secrets management more secure by being more automated.
Define your secrets as Kubernetes custom resources and external-secrets handles synchronization.
It can even reload your workloads when secrets change.
{{% /blocks/lead %}}

{{% blocks/section color="primary bg-gradient" type="row" %}}

{{% blocks/feature icon="fa-solid fa-vault" title="Connect to 30+ providers!" url="eso-docs/TODO" %}}
AWS Secrets Manager, HashiCorp Vault, Azure Key Vault, GCP Secret Manager, and 30+ more providers out of the box.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-arrows-rotate" title="Bi-directional Sync" url="eso-docs/TODO" %}}
Sync secrets from external stores into Kubernetes, or push Kubernetes secrets to external providers.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-robot" title="On-demand secret generation" url="eso-docs/TODO" %}}
Automatically generate passwords, SSH keys, container registry credentials, and more with built-in generators.
{{% /blocks/feature %}}

{{% /blocks/section %}}


{{% blocks/section color="secondary" type="row text-center" %}}
<h1>Projects</h1>
<p>&nbsp;</p>


{{% blocks/feature icon="fa-solid fa-brain" title="eso" %}}
Our main repository
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-repeat" title="reloader" %}}
A controller reloading your Resources in Cluster based on dynamic Events.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-terminal" title="esoctl" %}}
A CLI utility
{{% /blocks/feature %}}
{{% /blocks/section %}}

{{% blocks/section color="white" type="row text-center h1" %}}
Trusted by

[![cs-logo](/img/cs_logo.png)](https://container-solutions.com)
[![Form3](./img/form3_logo.png)](https://www.form3.tech/)
[![Pento](./img/pento_logo.png)](https://www.pento.io)
[![godaddy-logo](./img/godaddy_logo.png)](https://www.godaddy.com)

{{% /blocks/section %}}




{{% blocks/section color="secondary" type="row text-center h3" %}}

**External-Secrets is a [CNCF][] [sandbox][] project**

[![CNCF logo][]][cncf]

[cncf]: https://cncf.io
[cncf logo]: /img/cncf-white.svg
[sandbox]: https://www.cncf.io/projects/

{{% /blocks/section %}}
