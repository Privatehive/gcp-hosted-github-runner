locals {
  webhook_enterprise_url = local.hasEnterprise ? [format("Enterprise webhook Secret:   %s Payload URL %s/%s?%s=%s", random_password.webhook_enterprise_secret.result, google_cloud_run_v2_service.autoscaler.uri, local.webhookUrl, local.sourceQueryParamName, urlencode(var.github_enterprise))] : []
  webhook_org_url        = local.hasOrg ? [format("Organization webhook Secret: %s Payload URL %s/%s?%s=%s", random_password.webhook_org_secret.result, google_cloud_run_v2_service.autoscaler.uri, local.webhookUrl, local.sourceQueryParamName, urlencode(var.github_organization))] : []
  webhook_repos_urls     = local.hasRepo ? [for i, v in var.github_repositories : format("Repository webhook Secret:   %s Payload URL %s/%s?%s=%s", random_password.webhook_repo_secret[v].result, google_cloud_run_v2_service.autoscaler.uri, local.webhookUrl, local.sourceQueryParamName, urlencode(v))] : []
}

output "runner_webhook_config" {
  value       = join("\n", local.webhook_enterprise_url, local.webhook_org_url, local.webhook_repos_urls)
  description = "Create a webhook (for event: Workflow jobs) in your organization with this url and the given secret"
}

output "github_pat_secret_name" {
  value       = google_secret_manager_secret.github_pat_token.secret_id
  description = "The name of the secret where the GitHub Fine-grained personal access token has to be saved"
}

output "runner_registration_procedure" {
  value       = var.github_runner_group_id > 0 ? "jit-config registration" : "registration token"
  description = "What runner registration procedure is used"
}
