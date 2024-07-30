output "runner_webhook_config" {
  value       = "Payload URL: ${google_cloud_run_v2_service.autoscaler.uri}${local.webhookUrl} Secret: ${random_password.webhook_secret.result} Content type: application/json Events: Workflow jobs"
  description = "Create a webhook (for event: Workflow jobs) in your organization with this url and the given secret"
}
