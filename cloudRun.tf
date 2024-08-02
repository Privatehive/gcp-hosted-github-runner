resource "random_password" "webhook_secret" {
  length  = 24
  special = true
}

resource "google_cloud_run_v2_service" "autoscaler" {
  location   = local.region
  name       = "github-runner-autoscaler"
  ingress    = "INGRESS_TRAFFIC_ALL"
  depends_on = [google_artifact_registry_repository.ghcr, google_project_service.cloud_run_api]

  template {
    service_account                  = google_service_account.autoscaler_sa.email
    max_instance_request_concurrency = 20
    timeout                          = "120s"
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      image = "${local.region}-docker.pkg.dev/${local.projectId}/${google_artifact_registry_repository.ghcr.name}/privatehive/github-runner-autoscaler:latest"
      env {
        name  = "ROUTE_WEBHOOK"
        value = local.webhookUrl
      }
      env {
        name  = "WEBHOOK_SECRET"
        value = random_password.webhook_secret.result
      }
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
        value = google_cloud_tasks_queue.autoscaler_tasks.id
      }
      env {
        name  = "INSTANCE_TEMPLATE"
        value = google_compute_instance_template.runner_instance.id
      }
      env {
        name  = "SECRET_VERSION"
        value = "${google_secret_manager_secret.github_pat_token.id}/versions/latest"
      }
      env {
        name  = "RUNNER_PREFIX"
        value = var.github_runner_prefix
      }
      env {
        name  = "RUNNER_GROUP_NAME"
        value = var.github_runner_group_name
      }
      env {
        name  = "RUNNER_GROUP_ID"
        value = var.github_runner_group_id
      }
      env {
        name  = "RUNNER_LABELS"
        value = local.runnerLabel
      }
      env {
        name  = "GITHUB_ORG"
        value = var.github_organization
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
