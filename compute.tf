resource "google_compute_instance_template" "runner_instance" {

  name         = "ephemeral-github-runner"
  region       = local.region
  machine_type = var.machine_type
  tags         = var.enable_ssh ? ["http-egress", "ssh-ingress"] : ["http-egress"]
  depends_on   = [google_project_service.compute_api]

  scheduling {
    preemptible                 = var.machine_preemtible
    automatic_restart           = false
    on_host_maintenance         = "TERMINATE"
    instance_termination_action = "STOP"
    provisioning_model          = var.machine_preemtible ? "SPOT" : "STANDARD"
  }

  disk {
    auto_delete  = true
    boot         = true
    source_image = var.machine_image
    disk_type    = "pd-standard"
    disk_size_gb = 40
  }

  service_account {
    email  = google_service_account.github_runner_sa.email
    scopes = ["cloud-platform"]
  }

  network_interface {
    network    = google_compute_network.vpc_network.name
    subnetwork = google_compute_subnetwork.subnetwork.name

    dynamic "access_config" {
      for_each = var.use_cloud_nat ? [] : [0]
      content {
        network_tier = "STANDARD"
      }
    }
  }
}

resource "google_compute_project_metadata_item" "startup_scripts_register_runner" {
  key   = "startup_script_register_runner"
  value = <<EOT
#!/bin/bash
echo "Setup of agent '$(hostname)' started"
apt-get update && apt-get -y install docker.io docker-buildx curl
useradd -d /home/agent -u 10000 agent
usermod -aG docker agent
newgrp docker
wget -q -O /tmp/agent.tar.gz '${var.github_runner_download_url}'
mkdir -p /home/agent
chown -R agent:agent /home/agent
pushd /home/agent
sudo -u agent tar zxf /tmp/agent.tar.gz
registration_token=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/attributes/registration_token" -H "Metadata-Flavor: Google")
sudo -u agent ./config.sh --unattended --disableupdate --ephemeral --name $(hostname) ${local.runnerLabelInstanceTemplate} --url 'https://github.com/${var.github_organization}' --token $${registration_token} --runnergroup '${var.github_runner_group}' || shutdown now
./bin/installdependencies.sh || shutdown now
./svc.sh install agent || shutdown now
./svc.sh start || shutdown now
popd
rm /tmp/agent.tar.gz
echo "Setup finished"
EOT
}
