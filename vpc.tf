resource "google_compute_network" "vpc_network" {
  name                    = "spot-runner-network"
  description             = "The network the spot runner will join"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.compute_api]
}

resource "google_compute_subnetwork" "subnetwork" {
  name                     = "spot-runner-subnetwork"
  description              = "The subnetwork the spot runner will join"
  ip_cidr_range            = "10.0.1.0/24"
  network                  = google_compute_network.vpc_network.name
  private_ip_google_access = true
}
