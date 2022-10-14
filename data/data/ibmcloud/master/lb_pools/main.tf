locals {
  port_kubernetes_api = 6443
}

############################################
# Load balancer backend pool members
############################################

resource "ibm_is_lb_pool_member" "kubernetes_api_public" {
  count = var.public_endpoints ? var.control_plane_count : 0

  lb             = var.lb_kubernetes_api_public_id
  pool           = var.lb_pool_kubernetes_api_public_id
  port           = local.port_kubernetes_api
  target_address = var.control_plane_ips[count.index]
}

resource "ibm_is_lb_pool_member" "kubernetes_api_private" {
  count = var.control_plane_count

  lb             = var.lb_kubernetes_api_private_id
  pool           = var.lb_pool_kubernetes_api_private_id
  port           = local.port_kubernetes_api
  target_address = var.control_plane_ips[count.index]
}
