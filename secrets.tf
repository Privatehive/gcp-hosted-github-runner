resource "google_secret_manager_secret" "github_pat_token" {
  secret_id = "github-pat-token"
  depends_on = [google_project_service.secretmanager_api]

  replication {
    user_managed {
      replicas {
        location = local.region
      }
    }
  }
}
