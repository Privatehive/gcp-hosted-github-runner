resource "google_artifact_registry_repository" "ghcr" {
  location      = local.region
  repository_id = "ghcr"
  description   = "GitHub Container Registry (ghcr)"
  format        = "DOCKER"
  mode          = "REMOTE_REPOSITORY"
  depends_on    = [google_project_service.artifactregistry_api]

  remote_repository_config {
    description = "GitHub Container Registry (ghcr)"
    docker_repository {
      custom_repository {
        uri = "https://ghcr.io"
      }
    }
  }

  cleanup_policies {
    id     = "keep-one-version"
    action = "KEEP"
    most_recent_versions {
      keep_count = 1
    }
  }
}
