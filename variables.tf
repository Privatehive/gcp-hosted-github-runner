variable "machine_type" {
  type        = string
  description = "The VM instance machine type where the GitHub runner will run on."
  default     = "e2-micro"
}

variable "machine_image" {
  type        = string
  description = "The VM instance boot image (gcloud compute images list --filter ubuntu-os). Only Linux is supported!"
  default     = "ubuntu-os-cloud/ubuntu-minimal-2004-lts"
}

variable "machine_preemtible" {
  type        = bool
  description = "The VM instance will be an preemtible spot instance that costs much less but may be stopped by gcp at any time (leading to a failed workflow job)."
  default     = true
}

variable "machine_timeout" {
  type        = number
  description = "The maximum time a VM may run. Pick a number that is well outside the expected runner job timeouts but small enough to prevent unnecessary cost if a webhook event was lost or was not processed."
  default     = 14400 // 4 h
}

variable "enable_ssh" {
  type        = bool
  description = "Enable SSH access to the VM instances."
  default     = false
}

variable "use_cloud_nat" {
  type        = bool
  description = "Use a cloud NAT and router instead of a public ip address for the VM instances."
  default     = false
}

variable "enable_debug" {
  type        = bool
  description = "Enable debug messages of github-runner-autoscaler Cloud Run (WARNING: secrets will be leaked in log files)."
  default     = false
}

variable "github_enterprise" {
  type        = string
  description = "The name of the GitHub enterprise the runner will join."
  default     = ""
}

variable "github_organization" {
  type        = string
  description = "The name of the GitHub organization the runner will join."
  default     = ""
}

variable "github_repositories" {
  type        = list(string)
  description = "The name(s) of GitHub repositories the runner will join. The format of the repository is: OWNER/REPO."
  default     = []
}

variable "github_runner_group_id" {
  type        = number
  description = "The ID of the GitHub runner group the runner will join."
  default     = 1
}

variable "github_runner_labels" {
  type        = list(string)
  description = "One or multiple labels the runner will be tagged with."
  default     = ["self-hosted"]
  validation {
    condition     = length(var.github_runner_labels) > 0
    error_message = "The variable github_runner_labels must contain at least one value!"
  }
}

variable "github_runner_prefix" {
  type        = string
  description = "The name prefix of the runner (a random string will be automatically added to make the name unique)."
  default     = "runner"
}

variable "github_runner_download_url" {
  type        = string
  description = "The download link pointing to the gitlab runner package"
  default     = "https://github.com/actions/runner/releases/download/v2.321.0/actions-runner-linux-x64-2.321.0.tar.gz"
}

variable "github_runner_uid" {
  type        = number
  description = "The uid the runner will be run with."
  default     = 10000
}

variable "github_runner_packages" {
  type        = list(string)
  description = "Additional packages that will be installed in the runner with apt."
  default     = []
}

variable "force_cloud_run_deployment" {
  type        = bool
  description = "Use only for development. Each Terraform apply leads to a new revision of the cloud run."
  default     = false
}
