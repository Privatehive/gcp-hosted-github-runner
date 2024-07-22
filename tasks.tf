resource "google_cloud_tasks_queue" "agent_autoscaler_tasks" {
  name       = "autoscaler-callback-queue"
  location   = local.region
  depends_on = [google_project_service.cloudtasks_api]

  retry_config {
    max_attempts       = 10 // max_attempts && max_retry_duration have to be fullfilled to stop retires
    max_retry_duration = "3600s" // max_attempts && max_retry_duration have to be fullfilled to stop retires
    max_backoff        = "600s"
    min_backoff        = "60s"
    max_doublings      = 4
  }

  rate_limits {
    max_concurrent_dispatches = 1
    max_dispatches_per_second = 1
  }
}
