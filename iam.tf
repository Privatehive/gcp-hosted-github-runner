#data "google_compute_default_service_account" "default_sa" {
#}

#resource "google_service_account" "webhook_scheduler_sa" {
#  account_id   = "autoscaler-scheduler-sa"
#  display_name = "Invoke autoscaler"
#}

// Allow cloud run to pull image from container registry
resource "google_project_iam_member" "cloud_run_member" {
  project  = local.projectId
  member   = "serviceAccount:service-${data.google_project.current.number}@serverless-robot-prod.iam.gserviceaccount.com"
  for_each = toset(["roles/artifactregistry.reader"])
  role     = each.key
}

// ---- agent-autoscaler-sa ----
resource "google_service_account" "agent_autoscaler" {
  account_id   = "agent-autoscaler-sa"
  display_name = "Scales compute instances"
}

resource "google_project_iam_custom_role" "start_stop_agent_spot" {
  role_id     = "StartStopInstances"
  title       = "Start/Stop instance(s)"
  permissions = ["compute.instances.get", "compute.instances.start", "compute.instances.stop", "compute.instances.delete"]
}

resource "google_project_iam_member" "agent_autoscaler_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.start_stop_agent_spot.id
  condition {
    title      = "Instance startsWith ${var.github_runner_prefix}"
    description = "Allow Start/Stop only for instances starting with a resource name of: ${var.github_runner_prefix}"
    expression = "resource.name.startsWith('${var.github_runner_prefix}')"
  }
}
// -----------------------------

// If "allUsers" within member, allows public access. This will not work if organization policy "Domain Restricted Sharing" is active in project
resource "google_cloud_run_service_iam_binding" "public_access" {
  location = google_cloud_run_v2_service.agent_autoscaler.location
  service  = google_cloud_run_v2_service.agent_autoscaler.name
  role     = "roles/run.invoker"
  members = [
    "allUsers",
    //"serviceAccount:${google_service_account.webhook_scheduler_sa.email}"
  ]
}
