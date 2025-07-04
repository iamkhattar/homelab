# data "template_file" "worker-node-config" {
#   template  = file("${path.module}/config/cloud-init-worker.yaml")
#   vars      = {
#     local_ssh_public_key = file("${path.module}/.ssh/local_rsa.pub")
#     worker_ssh_public_key = tls_private_key.worker-ssh-key.public_key_openssh
#     worker_ssh_private_key = base64encode(tls_private_key.worker-ssh-key.private_key_openssh)
#   }
# }
#
# resource "hcloud_server" "worker-nodes" {
#   count = var.worker_node_server_count
#
#   # The name will be worker-node-0, worker-node-1, worker-node-2...
#   name        = "worker-node-${count.index}"
#   image       = var.server_image
#   server_type = var.worker_node_server_type
#   location    = var.cluster_location
#   public_net {
#     ipv4_enabled = true
#     ipv6_enabled = true
#   }
#   network {
#     network_id = hcloud_network.private_network.id
#   }
#   depends_on = [hcloud_network_subnet.private_network_subnet, hcloud_server.master-node]
# }