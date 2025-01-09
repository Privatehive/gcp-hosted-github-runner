locals {
  github_runner_package_install = join(" ", var.github_runner_packages)
}

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
    instance_termination_action = "DELETE"
    provisioning_model          = var.machine_preemtible ? "SPOT" : "STANDARD"

    max_run_duration {
      seconds = var.machine_timeout
    }
  }

  disk {
    auto_delete  = true
    boot         = true
    source_image = var.machine_image
    disk_type    = var.disk_type
    disk_size_gb = var.disk_size_gb
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

// First parameter has to be the registration token
/*
resource "google_compute_project_metadata_item" "startup_scripts_register_runner" {
  key   = "startup_script_register_runner"
  value = <<EOT
#!/bin/bash
echo "Setup of agent '$(hostname)' started"
apt-get update && apt-get -y install docker.io docker-buildx curl
useradd -d /home/agent -u ${var.github_runner_uid} agent
usermod -aG docker agent
newgrp docker
curl -s -o /tmp/agent.tar.gz -L '${var.github_runner_download_url}'
mkdir -p /home/agent
chown -R agent:agent /home/agent
pushd /home/agent
sudo -u agent tar zxf /tmp/agent.tar.gz
registration_token=$1
sudo -u agent ./config.sh --unattended --disableupdate --ephemeral --name $(hostname) ${local.runnerLabelInstanceTemplate} --url 'https://github.com/${var.github_organization}' --token $${registration_token} --runnergroup '${var.github_runner_group_name}' || shutdown now
./bin/installdependencies.sh || shutdown now
./svc.sh install agent || shutdown now
./svc.sh start || shutdown now
popd
rm /tmp/agent.tar.gz
echo "Setup finished"
EOT
}*/

// First parameter has to be the base64 encoded jit_config
resource "google_compute_project_metadata_item" "startup_scripts_register_jit_runner" {
  key   = "startup_script_register_jit_runner"
  value = <<EOT
#!/bin/bash
agent_name=$(hostname)
echo "Setup of agent '$agent_name' started"
apt-get update && apt-get -y install docker.io docker-buildx curl jq ${local.github_runner_package_install}
useradd -d /home/agent -u ${var.github_runner_uid} agent
usermod -aG docker agent
newgrp docker
RUNNER_DOWNLOAD_URL='${var.github_runner_download_url}'
if [ -z "$${RUNNER_DOWNLOAD_URL}" ]; then
  RUNNER_VERSION=$(curl -s "https://github.com/actions/runner/tags/" | grep -Eo "$Version v[0-9]+.[0-9]+.[0-9]+" | sort -r | head -n1 | tr -d ' ' | tr -d 'v')
  echo "Downloading latest runner v$${RUNNER_VERSION}"
  RUNNER_DOWNLOAD_URL="https://github.com/actions/runner/releases/download/v$${RUNNER_VERSION}/actions-runner-linux-x64-$${RUNNER_VERSION}.tar.gz"
fi
curl -s -o /tmp/agent.tar.gz -L $${RUNNER_DOWNLOAD_URL}
mkdir -p /home/agent
chown -R agent:agent /home/agent
pushd /home/agent
sudo -u agent tar zxf /tmp/agent.tar.gz
encoded_jit_config=$1
echo -n $encoded_jit_config | base64 -d | jq '.".runner"' -r | base64 -d > .runner
echo -n $encoded_jit_config | base64 -d | jq '.".credentials"' -r | base64 -d > .credentials
echo -n $encoded_jit_config | base64 -d | jq '.".credentials_rsaparams"' -r | base64 -d > .credentials_rsaparams
sed -i 's/{{SvcNameVar}}/actions.runner.service/g' bin/systemd.svc.sh.template
sed -i 's/{{SvcDescription}}/GitHub Actions Runner/g' bin/systemd.svc.sh.template
cp bin/systemd.svc.sh.template ./svc.sh && chmod +x ./svc.sh
./bin/installdependencies.sh || shutdown now
./svc.sh install agent || shutdown now
./svc.sh start || shutdown now
popd
rm /tmp/agent.tar.gz
echo "Setup finished - waiting for Workflow Job"
sleep 60
journalctl -u actions.runner.service -o json --no-pager | jq -e '.|.MESSAGE|match("Running job:")' || shutdown now
echo "Accepted Workflow Job - processing"
EOT
}
