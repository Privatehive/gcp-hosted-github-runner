variable "spot_machine_type" {
  type        = string
  description = "The machine type that each spot agent will use"
  default     = "e2-micro"
}

variable "spot_machine_image" {
  type        = string
  description = "The machine Linux image to run (gcloud compute images list --filter ubuntu-os)"
  default     = "ubuntu-os-cloud/ubuntu-minimal-2004-lts"
}

variable "enable_ssh" {
  type        = bool
  description = "Enable SSH access"
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

variable "github_runner_prefix" {
  type        = string
  description = "The prefix of each runner"
  default     = "runner"
}

variable "github_runner_download_url" {
  type        = string
  description = "The download link pointing to the gitlab runner package"
  default     = "https://github.com/actions/runner/releases/download/v2.317.0/actions-runner-linux-x64-2.317.0.tar.gz"
}
