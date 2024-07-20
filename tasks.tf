resource "google_cloud_tasks_queue" "agent_autoscaler_tasks" {
  name       = "autoscaler-webhook-queue"
  location   = local.region
  depends_on = [google_project_service.cloudtasks_api]

  retry_config {
    max_attempts       = 3
    max_retry_duration = "400s"
    max_backoff        = "120s"
    min_backoff        = "60s"
    max_doublings      = 1
  }

  rate_limits {
    max_concurrent_dispatches = 1
    max_dispatches_per_second = 1
  }

  timeouts {
    create = "120s"
  }
}
