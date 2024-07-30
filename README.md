# gcp-hosted-github-runner

[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Privatehive/gcp-hosted-github-runner/main.yml?branch=master&style=flat&logo=github&label=Docker+build)](https://github.com/Privatehive/gcp-hosted-github-runner/actions?query=branch%3Amaster)

**This terraform module provides a ready to use solution for Google Cloud hosted [GitHub ephemeral runner](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/autoscaling-with-self-hosted-runners#using-ephemeral-runners-for-autoscaling).**

> [!IMPORTANT]
> I am not responsible if this Terraform module results in high costs on your billing account. Keep an eye on your billing account and activate alerts!

## Quickstart

Create a [Fine-grained personal access token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-fine-grained-personal-access-token) with the Organization permission "Self-hosted runners". This is needed to automatically create a token for each ephemeral runner to join the runner group of the organization (**Warning**: The PAT will be visible in the startup script of the compute instance).

Add this Terraform module to your root module and provide the missing values:

``` hcl
module "spot-runner" {
  source               = "github.com/Privatehive/gcp-hosted-github-runner"
  github_pat_token     = "<personal_access_token>"
  github_organization  = "<the_organization>"
  github_runner_group  = "Default"
  github_runner_prefix = "runner"
  machine_type         = "c2d-highcpu-8"
}

provider "google" {
  project = "<gcp_project>"
  region  = "<gcp_region>"
  zone    = "<gcp_zone>"
}

output "runner_webhook_config" {
  value = nonsensitive(module.spot-runner.runner_webhook_config)
}
```

Authenticate with `gcloud` and apply the terraform module (apply twice if the first apply results in an error - wait some minutes in between)

``` bash
$ gcloud auth application-default login --project <gcp_project>
$ terraform init -upgrade && terraform apply
```

Have a look at the Terraform output `runner_webhook_config`. There you find the Cloud Run webhook url and secret. Now switch to your GitHub organization settings and create a new webhook:
* Fill in the Payload URL (from the Terraform output)
* Select Content type "application/json"
* Fill in the Secret (from the Terraform output)
* Enable SSL verification
* Select "Let me select individual events":
  * Make sure everything is deselected and then select "Workflow jobs" (at the bottom)
* Check "Active"
* Click "Add webhook"

That's it.

As soon as you start a GitHub workflow, which contains a job with `runs-on: self-hosted`, a VM instance (with the specified `machine_type`) starts. The name of the VM instance starts with the `github_runner_prefix`, which is followed by a random string to make the name unique. The name of the VM instance is also the name of the runner in the GitHub runner group. After the workflow job completed, the VM instance will be deleted again.

## Configuration

Have a look at the variables.tf file how to configure the Terraform module.

> [!TIP]
> To find the cheapest VM machine_type use this [table](https://gcloud-compute.com/instances.html) and sort by Spot instance cost. But remember that the price varies depending on the region.

## How it works

1. As soon as a new GitHub workflow job is queued, the GitHub webhook event "Workflow jobs" invokes the Cloud Run [container](https://github.com/Privatehive/gcp-hosted-github-runner/pkgs/container/runner-autoscaler) with path `/webhook`
2. The Cloud run enqueues a "create-vm" Cloud Task. This is necessary, because the timeout of a GitHub webhook is only 10 seconds but to start a VM instance takes about 1 minute.
3. The Cloud task invokes the Cloud Run path `/create_vm`.
4. The Cloud Run creates the VM instance from the instance template (preemtible spot VM instance by default)
5. In the startup script of the VM instance the PAT is used to generate a runner token. With the token the VM registers itself as an **ephemeral** runner in the runner group and immediately starts working on the workflow job.
6. As soon as the workflow job completed, the GitHub webhook event "Workflow jobs" invokes the Cloud Run again.
7. The Cloud run enqueues a "delete-vm" Cloud Task. This is necessary, because the timeout of a GitHub webhook is only 10 seconds but to delete a VM instance takes about 1 minute.
8. The Cloud task invokes the Cloud Run path `/delete_vm`.
9. The Cloud Run deletes the VM instance.

## Troubleshooting

#### Public access to Cloud Run disallowed

The terraform error looks something like this:
```
Error applying IAM policy for cloudrun service "v1/projects/github-spot-runner/locations/us-east1/services/cloudrun-service": Error setting IAM policy for cloudrun service "v1/projects/github-spot-runner/locations/us-east1/services/cloudrun-service": googleapi: Error 400: One or more users named in the policy do not belong to a permitted customer,  perhaps due to an organization policy
```

1. Solution: Use project tags: [How to create public Cloud Run services when Domain Restricted Sharing is enforced](https://cloud.google.com/blog/topics/developers-practitioners/how-create-public-cloud-run-services-when-domain-restricted-sharing-enforced?hl=en)

2. Solution: Override the Organization Policy "Domain Restricted Sharing" in the project, by setting it to "Allow all".

#### New VM Instance not starting (but a lot of instances are already running)

You exceeded your projects vCPU limit for the machine type in the region or for all regions. You may find an error log message in the Cloud Run logs stating `Machine Type vCPU quota exceeded for region`. Request a quota increase from google customer support for the project.
