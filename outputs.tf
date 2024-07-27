output "runner_webhook_url" {
  value       = "Payload URL: ${google_cloud_run_v2_service.agent_autoscaler.uri}${local.webhookUrl} Secret: ${random_password.webhook_secret.result} Content type: application/json Events: Workflow jobs"
  description = "Create a webhook (for event: Workflow jobs) in your organization with this url the secret"
}
