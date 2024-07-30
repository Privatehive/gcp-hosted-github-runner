variable "machine_type" {
  type        = string
  description = "The VM instance machine type where the GitHub runner will run on"
  default     = "e2-micro"
}

variable "machine_image" {
  type        = string
  description = "The VM instance boot image (gcloud compute images list --filter ubuntu-os). Only Linux is supported."
  default     = "ubuntu-os-cloud/ubuntu-minimal-2004-lts"
}

variable "machine_preemtible" {
  type        = bool
  description = "The VM instance will be an preemtible spot instance that costs much less but may be stop by gcp at any time (leading to a failed workflow job)."
  default     = true
}

variable "enable_ssh" {
  type        = bool
  description = "Enable SSH access to the VM instances"
  default     = false
}

variable "use_cloud_nat" {
  type        = bool
  description = "Use a cloud NAT and router instead of a public ip address for the VM instances"
  default     = false
}

variable "enable_debug" {
  type        = bool
  description = "Enable debug messages in agent-autoscaler (WARNING: secrets will be leaked in log files)"
  default     = false
}

variable "github_pat_token" {
  type        = string
  sensitive   = true
  description = "A PAT to get a runner registration token for the organization https://docs.github.com/en/rest/actions/self-hosted-runners?apiVersion=2022-11-28#create-a-registration-token-for-an-organization"
}

variable "github_organization" {
  type        = string
  description = "The name of the GitHub organization the runner will join"
}

variable "github_runner_group" {
  type        = string
  description = "The name of the GitHub runner group the runner will join"
  default     = "Default"
}

variable "github_runner_labels" {
  type        = list(string)
  description = "One or multiple labels the runner will be tagged with"
  default     = ["self-hosted"]
  validation {
    condition     = length(var.github_runner_labels) > 0
    error_message = "The variable github_runner_labels must contain at least one not empty value!"
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
  default     = "https://github.com/actions/runner/releases/download/v2.317.0/actions-runner-linux-x64-2.317.0.tar.gz"
}
