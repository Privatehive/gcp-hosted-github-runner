#data "google_compute_default_service_account" "default_sa" {
#}

#resource "google_service_account" "webhook_scheduler_sa" {
#  account_id   = "autoscaler-scheduler-sa"
#  display_name = "Invoke autoscaler"
#}

// Allow cloud run to pull image from container registry
resource "google_project_iam_member" "cloud_run_member" {
  project    = local.projectId
  member     = "serviceAccount:service-${data.google_project.current.number}@serverless-robot-prod.iam.gserviceaccount.com"
  for_each   = toset(["roles/artifactregistry.reader"])
  depends_on = [google_cloud_run_v2_service.agent_autoscaler]
  role       = each.key
}

// ---- agent-autoscaler-sa ----
resource "google_service_account" "agent_autoscaler" {
  account_id   = "agent-autoscaler-sa"
  display_name = "Scales compute instances"
}

resource "google_project_iam_custom_role" "start_stop_agent_spot" {
  role_id     = "StartStopInstances"
  title       = "Start/Stop instance(s)"
  permissions = ["compute.instances.get", "compute.instances.start", "compute.instances.stop", "compute.instances.delete", "compute.instances.create", "compute.instances.setMetadata", "compute.instances.setTags"]
}

resource "google_project_iam_custom_role" "create_task" {
  role_id     = "CreateTask"
  title       = "Create a Cloud Task"
  permissions = ["cloudtasks.tasks.create"]
}

resource "google_project_iam_custom_role" "create_from_instance_template" {
  role_id     = "CreateInstanceFromTemplate"
  title       = "Create an instance from template"
  permissions = ["compute.instanceTemplates.useReadOnly"]
}

resource "google_project_iam_custom_role" "create_disk" {
  role_id     = "CreateDisk"
  title       = "Create Disk"
  permissions = ["compute.disks.create"]
}

resource "google_project_iam_custom_role" "subnetwork_use" {
  role_id     = "SubnetworkUse"
  title       = "Subnetwork Use"
  permissions = ["compute.subnetworks.use", "compute.subnetworks.useExternalIp"]
}

resource "google_project_iam_member" "agent_autoscaler_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.start_stop_agent_spot.id
  condition {
    title       = "Instance startsWith ${var.github_runner_prefix}"
    description = "Allow Start/Stop only for instances starting with a resource name of: ${var.github_runner_prefix}"
    expression  = "resource.name.startsWith('projects/${local.projectId}/zones/${local.zone}/instances/${var.github_runner_prefix}-')"
  }
}

resource "google_project_iam_member" "create_task_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.create_task.id
  #condition {
  #  title      = "Cloud task resource name equals: ${google_cloud_tasks_queue.agent_autoscaler_tasks.name}"
  #  description = "Allow to create a task in the queue: ${google_cloud_tasks_queue.agent_autoscaler_tasks.name}"
  #  expression = "resource.name == '${google_cloud_tasks_queue.agent_autoscaler_tasks.id}'"
  #}
}

resource "google_project_iam_member" "create_from_instance_template_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.create_from_instance_template.id
  condition {
    title       = "Instance template: ${google_compute_instance_template.spot_instance.name}"
    description = "Allow to create an instance from template: ${google_compute_instance_template.spot_instance.id}"
    expression  = "resource.name == '${google_compute_instance_template.spot_instance.id}'"
  }
}

resource "google_project_iam_member" "create_disk_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.create_disk.id
  condition {
    title      = "Create disk"
    expression = "resource.name.startsWith('projects/${local.projectId}/zones/${local.zone}/disks/${var.github_runner_prefix}-')"
  }
}

resource "google_project_iam_member" "subnetwork_use_member" {
  project = local.projectId
  member  = "serviceAccount:${google_service_account.agent_autoscaler.email}"
  role    = google_project_iam_custom_role.subnetwork_use.id
  condition {
    title      = "Use Subnetwork"
    expression = "resource.name == '${google_compute_subnetwork.subnetwork.id}'"
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
