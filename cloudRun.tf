resource "random_password" "webhook_enterprise_secret" {
  length  = 24
  special = true
}

resource "random_password" "webhook_org_secret" {
  length  = 24
  special = true
}

resource "random_password" "webhook_repo_secret" {
  for_each = toset(var.github_repositories)
  length   = 24
  special  = true
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
      image = "${local.region}-docker.pkg.dev/${local.projectId}/${google_artifact_registry_repository.ghcr.name}/${local.runnerDockerImage}:${local.runnerDockerTag}"
      env {
        name  = "ROUTE_WEBHOOK"
        value = local.webhookUrl
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
        name  = "RUNNER_GROUP_ID"
        value = var.github_runner_group_id
      }
      env {
        name  = "RUNNER_LABELS"
        value = local.runnerLabel
      }
      env {
        name  = "GITHUB_ENTERPRISE"
        value = local.hasEnterprise ? format("%s;%s", var.github_enterprise, base64encode(random_password.webhook_enterprise_secret.result)) : ""
      }
      env {
        name  = "GITHUB_ORG"
        value = local.hasOrg ? format("%s;%s", var.github_organization, base64encode(random_password.webhook_org_secret.result)) : ""
      }
      env {
        name  = "GITHUB_REPOS"
        value = local.hasRepo ? join(",", [for i, v in var.github_repositories : format("%s;%s", v, base64encode(random_password.webhook_repo_secret[v].result))]) : ""
      }
      env {
        name  = "SOURCE_QUERY_PARAM_NAME"
        value = local.sourceQueryParamName
      }
      dynamic "env" {
        for_each = var.force_cloud_run_deployment ? [0] : []
        content {
          name  = "TIMESTAMP"
          value = timestamp()
        }
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
