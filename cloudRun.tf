resource "random_password" "auth_password" {
  length  = 16
  special = true
}

resource "google_cloud_run_v2_service" "agent_autoscaler" {
  location   = local.region
  name       = "runner-autoscaler"
  ingress    = "INGRESS_TRAFFIC_ALL"
  depends_on = [google_artifact_registry_repository.ghcr, google_project_service.cloud_run_api]

  template {
    service_account                  = google_service_account.agent_autoscaler.email
    max_instance_request_concurrency = 11
    timeout                          = "120s"
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      image = "${local.region}-docker.pkg.dev/${local.projectId}/${google_artifact_registry_repository.ghcr.name}/tereius/runner-autoscaler:latest"
      env {
        name  = "PROJECT_ID"
        value = local.projectId
      }
      env {
        name  = "ZONE"
        value = local.zone
      }
      env {
        name  = "TASK_QUEUE"
        value = google_cloud_tasks_queue.agent_autoscaler_tasks.id
      }
      env {
        name  = "INSTANCE_TEMPLATE_URL"
        value = google_compute_instance_template.spot_instance.id
      }
      env {
        name  = "RUNNER_PREFIX"
        value = var.github_runner_prefix
      }
      env {
        name  = "WEBHOOK_SECRET"
        value = random_password.auth_password.result
      }
      env {
        name  = "ROUTE_WEBHOOK"
        value = local.webhookUrl
      }
      env {
        name  = "ROUTE_CREATE_RUNNER"
        value = local.webhookCreateRunner
      }
      env {
        name  = "ROUTE_DELETE_RUNNER"
        value = local.webhookDeleteRunner
      }
      dynamic "env" {
        for_each = var.enable_debug ? [0] : []
        content {
          name  = "DEBUG"
          value = 1
        }
      }
      resources {
        startup_cpu_boost = false
        cpu_idle          = true
        limits = {
          cpu    = "1"
          memory = "128Mi"
        }
      }
    }
  }
}
