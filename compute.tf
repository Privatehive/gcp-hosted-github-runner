resource "google_compute_instance_template" "spot_instance" {

  name         = "ephemeral-runner"
  region       = local.region
  machine_type = var.spot_machine_type
  tags         = ["http-egress", "ssh-ingress"]
  depends_on   = [google_project_service.compute_api]

  scheduling {
    preemptible                 = true
    automatic_restart           = false
    on_host_maintenance         = "TERMINATE"
    instance_termination_action = "STOP"
    provisioning_model          = "SPOT"
  }

  disk {
    auto_delete  = true
    boot         = true
    source_image = var.spot_machine_image
    disk_type    = "pd-standard"
    disk_size_gb = 30
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

  # show log: sudo journalctl -u google-startup-scripts.service
  # run again: sudo google_metadata_script_runner startup
  metadata_startup_script = <<EOT
#!/bin/bash
echo "Setup of agent '$(hostname)' started"
apt-get update && apt-get -y install docker.io docker-buildx jq curl
useradd -d /home/agent -u 10000 agent
usermod -aG docker agent
newgrp docker
wget -q -O /tmp/agent.tar.gz '${var.github_runner_download_url}'
mkdir -p /home/agent
chown -R agent:agent /home/agent
pushd /home/agent
sudo -u agent tar zxf /tmp/agent.tar.gz
register_token=$(curl -s -L -X POST -H "Accept: application/vnd.github+json" -H "Authorization: Bearer ${var.github_pat_token}" -H "X-GitHub-Api-Version: 2022-11-28" https://api.github.com/orgs/${var.github_organization}/actions/runners/registration-token | jq -r .token)
sudo -u agent ./config.sh --unattended --disableupdate --ephemeral --name $(hostname) --url 'https://github.com/${var.github_organization}' --token $${register_token} --runnergroup '${var.github_runner_group}' || shutdown now
./bin/installdependencies.sh || shutdown now
./svc.sh install agent || shutdown now
./svc.sh start || shutdown now
popd
rm /tmp/agent.tar.gz
echo "Setup finished"
EOT
}
