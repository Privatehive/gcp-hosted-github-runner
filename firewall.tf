resource "google_compute_firewall" "http-egress" {
  name    = "http-egress"
  description = "Allows egress on port 80, 443"
  network = google_compute_network.vpc_network.name
  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }

  target_tags = ["http-egress"]
}

resource "google_compute_firewall" "ssh-ingress" {
  name    = "ssh-ingress"
  description = "Allows ingress on port 22"
  network = google_compute_network.vpc_network.name
  direction = "INGRESS"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags = ["ssh-ingress"]
}
