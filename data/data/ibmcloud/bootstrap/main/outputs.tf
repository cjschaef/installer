output "bootstrap_floating_ip" {
  value = var.public_endpoints ? ibm_is_floating_ip.bootstrap_floatingip[0].address : null
}

output "bootstrap_ip" {
  value = ibm_is_instance.bootstrap_node.primary_network_interface[0].primary_ipv4_address
}
