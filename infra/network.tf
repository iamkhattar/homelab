resource "hcloud_network" "private_network" {
  name     = "k3s-cluster"
  ip_range = var.networking.private_network_cidr
}

resource "hcloud_network_subnet" "private_network_subnet" {
  network_id   = hcloud_network.private_network.id
  type         = "cloud"
  network_zone = var.networking.private_subnet_zone
  ip_range     = var.networking.private_subnet_cidr
}