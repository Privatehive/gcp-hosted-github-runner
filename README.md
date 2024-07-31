# gcp-hosted-github-runner

[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Privatehive/gcp-hosted-github-runner/main.yml?branch=master&style=flat&logo=github&label=Docker+build)](https://github.com/Privatehive/gcp-hosted-github-runner/actions?query=branch%3Amaster)

**This terraform module provides a ready to use solution for Google Cloud hosted [GitHub ephemeral runner](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/autoscaling-with-self-hosted-runners#using-ephemeral-runners-for-autoscaling).**

> [!IMPORTANT]
> I am not responsible if this Terraform module results in high costs on your billing account. Keep an eye on your billing account and activate alerts!

> [!NOTE]
> Only works for GitHub organization at the moment.

## Quickstart

#### 1. Apply Terraform
Add this Terraform module to your root module and provide the missing values:

``` hcl
provider "google" {
  project = "<gcp_project>"
  region  = "<gcp_region>"
  zone    = "<gcp_zone>"
}

module "github-runner" {
  source               = "github.com/Privatehive/gcp-hosted-github-runner"
  github_organization  = "<the_organization>" // Provide the name of the GitHub Organization
  github_runner_group  = "Default" // The name of the GitHub Organization runner group
  github_runner_prefix = "runner" // The VM instance name starts with this prefix (a random string is added as a suffix)
  machine_type         = "c2d-highcpu-8" // The machine type of the VM instance
}

output "runner_webhook_config" {
  value = nonsensitive(module.github-runner.runner_webhook_config) // remove the output after the initial setup
}
```

Authenticate with `gcloud` and apply the terraform module (apply twice if the first apply results in an error - wait some minutes in between)

``` bash
$ gcloud auth application-default login --project <gcp_project>
$ terraform init -upgrade && terraform apply
```

> [!IMPORTANT]
> After a successful initial setup you should remove the `runner_webhook_config` output because it prints the webhook secret. Also make sure that the Terraform state file is stored in a save place (e.g. in a [Cloud Storage bucket](https://cloud.google.com/docs/terraform/resource-management/store-state)). The state file contains the webhook secret as plaintext.

#### 2. Configure GitHub webhook

Have a look at the Terraform output `runner_webhook_config`. There you find the Cloud Run webhook url and secret. Now switch to your GitHub Organization settings and create a [new Organization webhook](https://docs.github.com/en/webhooks/using-webhooks/creating-webhooks#creating-an-organization-webhook):
* Fill in the Payload URL (from the Terraform output)
* Select Content type "application/json"
* Fill in the Secret (from the Terraform output)
* Enable SSL verification
* Select "Let me select individual events":
  * Make sure everything is deselected and then select "Workflow jobs" (at the bottom)
* Check "Active"
* Click "Add webhook"

#### 3. Provide PAT

Create a [Fine-grained personal access token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-fine-grained-personal-access-token) with the Organization Read/Write permission "Self-hosted runners". This PAT is needed to automatically create a shored lived [registration token](https://docs.github.com/en/rest/actions/self-hosted-runners?apiVersion=2022-11-28#create-a-registration-token-for-an-organization) for each ephemeral runner to join the runner group of the Organization.

Then open the [Secret Manager](https://console.cloud.google.com/security/secret-manager) in the Google Cloud Console and add a new Version to the already existing secret "github-pat-token". Paste the PAT into the Secret value field and click "ADD NEW VERSION".

That's it 👍

As soon as you start a GitHub workflow, which contains a job with `runs-on: self-hosted` (or any other label you provided to the Terraform [module variable](./variables.tf) `github_runner_labels`), a VM instance (with the specified `machine_type`) starts. The name of the VM instance starts with the `github_runner_prefix`, which is followed by a random string to make the name unique. The name of the VM instance is also the name of the runner in the GitHub runner group. After the workflow job completed, the VM instance will be deleted again.

## Advanced Configuration

Have a look at the [variables.tf](./variables.tf) file how to further configure the Terraform module.

> [!TIP]
> To find the cheapest VM machine_type use this [table](https://gcloud-compute.com/instances.html) and sort by Spot instance cost. But remember that the price varies depending on the region.

## How it works

1. As soon as a new GitHub workflow job is queued, the GitHub webhook event "Workflow jobs" invokes the Cloud Run [container](https://github.com/Privatehive/gcp-hosted-github-runner/pkgs/container/github-runner-autoscaler) with path `/webhook`
2. The Cloud run enqueues a "create-vm" Cloud Task. This is necessary, because the timeout of a GitHub webhook is only 10 seconds but to start a VM instance takes about 1 minute.
3. The Cloud task invokes the Cloud Run path `/create_vm`.
4. The Cloud Run creates the VM instance from the instance template (preemtible spot VM instance by default)
5. In the startup script of the VM instance the PAT is used to generate a runner token. With the token the VM registers itself as an **ephemeral** runner in the runner group and immediately starts working on the workflow job.
6. As soon as the workflow job completed, the GitHub webhook event "Workflow jobs" invokes the Cloud Run again.
7. The Cloud run enqueues a "delete-vm" Cloud Task. This is necessary, because the timeout of a GitHub webhook is only 10 seconds but to delete a VM instance takes about 1 minute.
8. The Cloud task invokes the Cloud Run path `/delete_vm`.
9. The Cloud Run deletes the VM instance.

> [!NOTE]
> The runner is run by the unprivileged user `agent` with the uid `10000` and gid `10000`

## Expected Cost

The following Google Cloud resources are created that may generate cost:
* Cloud Task (covered by Free Tier)
* Secret Version (covered by Free Tier)
* Artifact Registry (covered by Free Tier)
* Cloud Run (covered by Free Tier)
* (Spot) VM Instance(s) + standard persistent disk + ephemeral external IPv4

Other:
* Egress network traffic (200 GiB/month is free)

**Example:**

A single 1 h long workflow job in europe-west1 leads to the following cost:

```
Ephemeral external IPv4 for Spot instance $0.0025
Spot VM Instance c2d-highcpu-8            $0.0494
Standard persistent disk 20 GiB used    ~ $0.0011
-----------------------------------------------------
                                          $0.053
```

Overall, only the compute instance accounts for the majority of the costs.

## Troubleshooting

#### Public access to Cloud Run disallowed

The terraform error looks something like this:
```
Error applying IAM policy for cloudrun service "v1/projects/github-spot-runner/locations/us-east1/services/cloudrun-service": Error setting IAM policy for cloudrun service "v1/projects/github-spot-runner/locations/us-east1/services/cloudrun-service": googleapi: Error 400: One or more users named in the policy do not belong to a permitted customer, perhaps due to an Organization policy
```

1. Solution: Use project tags: [How to create public Cloud Run services when Domain Restricted Sharing is enforced](https://cloud.google.com/blog/topics/developers-practitioners/how-create-public-cloud-run-services-when-domain-restricted-sharing-enforced?hl=en)

2. Solution: Override the Organization Policy "Domain Restricted Sharing" in the project, by setting it to "Allow all".

#### The VM Instance immediately stops after it was created without processing a workflow job

The VM will shoutdown itself if the registration at the GitHub runner group fails. This can be caused by:
* An invalid/expired PAT or a PAT with insufficient permission. Check if the PAT is valid, has the Organization Read/Write permission "Self-hosted runners" and is stored in the Secret Manager secret.
* A typo in the GitHub Organization name. Check the Terraform variable `github_organization` for typos.
* A not existing GitHub runner group within the Organization. Check the Terraform variable `github_runner_group` for typos.

You can observer the runner registration process by connecting to the VM instance via SSH and running:
```
$ sudo journalctl -u google-startup-scripts.service --follow
```

#### New VM Instance not starting (but a lot of instances are already running)

You exceeded your projects vCPU limit for the machine type in the region or for all regions. You may find an error log message in the Cloud Run logs stating `Machine Type vCPU quota exceeded for region`. Request a quota increase from google customer support for the project.
