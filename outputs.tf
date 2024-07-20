output "azure_webhook_url" {
  value       = "${google_cloud_run_v2_service.agent_autoscaler.uri}${local.webhookUrl}"
  description = "The url of the Cloud Run webhook"
}
