locals {
  port_kubernetes_api = 6443
}

############################################
# Load balancer backend pool members
############################################

resource "ibm_is_lb_pool_member" "kubernetes_api_public" {
  count = var.public_endpoints ? 1 : 0

  lb             = var.lb_kubernetes_api_public_id
  pool           = var.lb_pool_kubernetes_api_public_id
  port           = local.port_kubernetes_api
  target_address = var.bootstrap_node_ip
}

resource "ibm_is_lb_pool_member" "kubernetes_api_private" {
  lb             = var.lb_kubernetes_api_private_id
  pool           = var.lb_pool_kubernetes_api_private_id
  port           = local.port_kubernetes_api
  target_address = var.bootstrap_node_ip
}
