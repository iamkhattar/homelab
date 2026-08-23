variables {
  # hcloud provider validates token length; it must be exactly 64 characters.
  hetzner_cloud_api_token = "0000000000000000000000000000000000000000000000000000000000000000"
  k3s_api_token           = "test-k3s-token"
  ssh_public_key          = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC test@example.com"
}

# Test default configuration with no agents
run "default_configuration" {
  command = plan

  assert {
    condition     = hcloud_network.private_network.ip_range == "10.0.0.0/16"
    error_message = "Private network CIDR should be 10.0.0.0/16 by default"
  }

  assert {
    condition     = hcloud_network_subnet.private_network_subnet.ip_range == "10.0.1.0/24"
    error_message = "Private subnet CIDR should be 10.0.1.0/24 by default"
  }

  assert {
    condition     = hcloud_network_subnet.private_network_subnet.network_zone == "eu-central"
    error_message = "Network zone should be eu-central by default"
  }

  assert {
    condition     = hcloud_server.server_node.name == "server-node-0"
    error_message = "Server node name should be server-node-0"
  }

  assert {
    condition     = hcloud_server.server_node.server_type == "cx43"
    error_message = "Server node type should be cx43 by default"
  }

  assert {
    condition     = hcloud_server.server_node.image == "debian-12"
    error_message = "Server node image should be debian-12 by default"
  }

  assert {
    condition     = hcloud_server.server_node.location == "nbg1"
    error_message = "Server node location should be nbg1 by default"
  }

  assert {
    condition     = length(hcloud_server.agent_nodes) == 0
    error_message = "No agent nodes should be created by default"
  }
}

# Test server node firewall rules
run "server_firewall_rules" {
  command = plan


  assert {
    condition     = length([for rule in hcloud_firewall.public_nodes_firewall.rule : rule if rule.port == "22"]) == 1
    error_message = "Public firewall should allow SSH on port 22"
  }

  assert {
    condition     = length([for rule in hcloud_firewall.public_nodes_firewall.rule : rule if rule.port == "80"]) == 1
    error_message = "Public firewall should allow HTTP on port 80"
  }

  assert {
    condition     = length([for rule in hcloud_firewall.public_nodes_firewall.rule : rule if rule.port == "443"]) == 1
    error_message = "Public firewall should allow HTTPS on port 443"
  }

  assert {
    condition     = length([for rule in hcloud_firewall.public_nodes_firewall.rule : rule if rule.port == "6443"]) == 1
    error_message = "Public firewall should allow K8s API on port 6443"
  }
}

# Test server node networking
run "server_networking" {
  command = plan

  assert {
    condition     = anytrue([for pn in hcloud_server.server_node.public_net : pn.ipv4_enabled])
    error_message = "Server node should have IPv4 enabled"
  }

  assert {
    condition     = anytrue([for pn in hcloud_server.server_node.public_net : pn.ipv6_enabled])
    error_message = "Server node should have IPv6 enabled"
  }

  assert {
    condition     = anytrue([for net in hcloud_server.server_node.network : net.ip == "10.0.1.1"])
    error_message = "Server node should have private IP 10.0.1.1 by default"
  }
}

# Test with custom agent count
run "with_agent_nodes" {
  command = plan

  variables {
    agent = {
      image = "debian-12"
      type  = "cx32"
      count = 3
    }
  }

  assert {
    condition     = length(hcloud_server.agent_nodes) == 3
    error_message = "Should create 3 agent nodes when count is set to 3"
  }

  assert {
    condition     = hcloud_server.agent_nodes[0].name == "agent-node-0"
    error_message = "First agent node should be named agent-node-0"
  }

  assert {
    condition     = hcloud_server.agent_nodes[2].name == "agent-node-2"
    error_message = "Third agent node should be named agent-node-2"
  }

  assert {
    condition     = alltrue([for node in hcloud_server.agent_nodes : node.server_type == "cx32"])
    error_message = "All agent nodes should be cx32 type by default"
  }

  assert {
    condition     = alltrue([for node in hcloud_server.agent_nodes : node.image == "debian-12"])
    error_message = "All agent nodes should use debian-12 image by default"
  }

  assert {
    condition     = alltrue([for node in hcloud_server.agent_nodes : node.location == "nbg1"])
    error_message = "All agent nodes should be in nbg1 location by default"
  }
}

# Test private firewall rules
run "private_firewall_rules" {
  command = plan

  assert {
    condition     = length([for rule in hcloud_firewall.private_nodes_firewall.rule : rule if rule.protocol == "tcp" && rule.port == "any"]) == 1
    error_message = "Private firewall should allow all TCP traffic from private network"
  }

  assert {
    condition     = length([for rule in hcloud_firewall.private_nodes_firewall.rule : rule if rule.protocol == "udp" && rule.port == "any"]) == 1
    error_message = "Private firewall should allow all UDP traffic from private network"
  }

  assert {
    condition     = length([for rule in hcloud_firewall.private_nodes_firewall.rule : rule if rule.protocol == "icmp"]) == 1
    error_message = "Private firewall should allow ICMP traffic from private network"
  }

  assert {
    condition     = alltrue([for rule in hcloud_firewall.private_nodes_firewall.rule : contains(rule.source_ips, "10.0.0.0/16")])
    error_message = "All private firewall rules should restrict to private network CIDR"
  }
}

# Test custom networking configuration
run "custom_networking" {
  command = plan

  variables {
    networking = {
      private_network_cidr = "192.168.0.0/16"
      private_subnet_zone  = "eu-central"
      private_subnet_cidr  = "192.168.1.0/24"
    }
    server = {
      image = "debian-12"
      type  = "cx43"
      ip    = "192.168.1.1"
    }
  }

  assert {
    condition     = hcloud_network.private_network.ip_range == "192.168.0.0/16"
    error_message = "Private network should use custom CIDR"
  }

  assert {
    condition     = hcloud_network_subnet.private_network_subnet.ip_range == "192.168.1.0/24"
    error_message = "Private subnet should use custom CIDR"
  }

  assert {
    condition     = anytrue([for net in hcloud_server.server_node.network : net.ip == "192.168.1.1"])
    error_message = "Server node should use custom private IP"
  }
}

# Test custom server configuration
run "custom_server_config" {
  command = plan

  variables {
    server = {
      image = "ubuntu-22.04"
      type  = "cx52"
      ip    = "10.0.1.10"
    }
    cluster = {
      location   = "fsn1"
      datacenter = "fsn1-dc14"
    }
  }

  assert {
    condition     = hcloud_server.server_node.image == "ubuntu-22.04"
    error_message = "Server should use custom image"
  }

  assert {
    condition     = hcloud_server.server_node.server_type == "cx52"
    error_message = "Server should use custom server type"
  }

  assert {
    condition     = anytrue([for net in hcloud_server.server_node.network : net.ip == "10.0.1.10"])
    error_message = "Server should use custom private IP"
  }

  assert {
    condition     = hcloud_server.server_node.location == "fsn1"
    error_message = "Server should use custom location"
  }
}

# Test resource labels
run "resource_labels" {
  command = plan

  variables {
    agent = {
      image = "debian-12"
      type  = "cx32"
      count = 1
    }
  }

  assert {
    condition     = hcloud_server.server_node.labels["type"] == "server"
    error_message = "Server node should have 'type: server' label"
  }

  assert {
    condition     = hcloud_server.agent_nodes[0].labels["type"] == "agent"
    error_message = "Agent nodes should have 'type: agent' label"
  }
}
