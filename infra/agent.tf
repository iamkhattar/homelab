data "template_file" "agent_node_config" {
  template = file("${path.module}/config/cloud-init-agent.yaml")
  vars = {
    local_ssh_public_key = file("${path.module}/config/.ssh/id_rsa.pub")
    k3s_api_token = var.k3s_api_token
  }
}

resource "hcloud_server" "agent_nodes" {
  count = var.agent.count

  # The name will be worker-node-0, worker-node-1, worker-node-2...
  name        = "agent-node-${count.index}"
  image       = var.agent.image
  server_type = var.agent.type
  location    = var.cluster.location
  labels = {
    type : "agent"
  }

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  firewall_ids = [hcloud_firewall.private_nodes_firewall.id]

  network {
    network_id = hcloud_network.private_network.id
  }

  user_data = data.template_file.agent_node_config.rendered

  depends_on = [hcloud_network_subnet.private_network_subnet, hcloud_server.server_node]
}