resource "random_string" "task_queue_suffix" {
  length  = 5
  upper   = false
  special = false
  numeric = false
}

resource "google_cloud_tasks_queue" "autoscaler_tasks" {
  name       = "autoscaler-callback-queue-${random_string.task_queue_suffix.result}"
  location   = local.region
  depends_on = [google_project_service.cloudtasks_api]

  retry_config {
    max_attempts       = 10      // max_attempts && max_retry_duration have to be fullfilled to stop retires
    max_retry_duration = "3600s" // max_attempts && max_retry_duration have to be fullfilled to stop retires
    max_backoff        = "600s"
    min_backoff        = "60s"
    max_doublings      = 4
  }

  rate_limits {
    max_concurrent_dispatches = var.max_concurrency
    max_dispatches_per_second = var.max_concurrency
  }
}
