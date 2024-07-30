resource "google_compute_network" "vpc_network" {
  name                    = "spot-runner-network"
  description             = "The network the ephemeral GitHub runner instances will join"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.compute_api]
}

resource "google_compute_subnetwork" "subnetwork" {
  name                     = "spot-runner-subnetwork"
  description              = "The subnetwork the ephemeral GitHub runner instances will join"
  ip_cidr_range            = "10.0.1.0/24"
  network                  = google_compute_network.vpc_network.name
  private_ip_google_access = true
}

resource "google_compute_router" "router" {
  count   = var.use_cloud_nat ? 1 : 0
  name    = "router"
  network = google_compute_network.vpc_network.id
}

resource "google_compute_router_nat" "nat" {
  count                              = var.use_cloud_nat ? 1 : 0
  name                               = "router-nat"
  router                             = google_compute_router.router[0].name
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES"
  auto_network_tier                  = "STANDARD"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}
