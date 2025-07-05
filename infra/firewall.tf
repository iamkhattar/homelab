resource "hcloud_firewall" "public_nodes_firewall" {
  name = "public-nodes-firewall"

  rule {
    description = "Allow HTTP traffic"
    direction   = "in"
    protocol    = "tcp"
    port        = "80"
    source_ips = [
      "0.0.0.0/0",
      "::/0"
    ]
  }

  rule {
    description = "Allow HTTPS traffic"
    direction   = "in"
    protocol    = "tcp"
    port        = "443"
    source_ips = [
      "0.0.0.0/0",
      "::/0"
    ]
  }

  rule {
    description = "Allow K8S API traffic"
    direction   = "in"
    protocol    = "tcp"
    port        = "6443"
    source_ips = [
      "0.0.0.0/0",
      "::/0"
    ]
  }

  # TODO: Remove this we shouldn't keep standing ssh access
  rule {
    description = "Allow SSH traffic"
    direction   = "in"
    protocol    = "tcp"
    port        = "22"
    source_ips = [
      "0.0.0.0/0",
      "::/0"
    ]
  }

}


resource "hcloud_firewall" "private_nodes_firewall" {
  name = "private-nodes-firewall"

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "any"
    source_ips = [var.networking.private_network_cidr]
    description = "Allow all tcp traffic from private network"
  }

  rule {
    direction   = "in"
    protocol    = "udp"
    port        = "any"
    source_ips = [var.networking.private_network_cidr]
    description = "Allow all tcp traffic from private network"
  }

  rule {
    direction   = "in"
    protocol    = "icmp"
    source_ips = [var.networking.private_network_cidr]
    description = "Allow all icmp traffic from private network"
  }
}
