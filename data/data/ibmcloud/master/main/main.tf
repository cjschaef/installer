locals {
  prefix              = var.cluster_id
  port_machine_config = 22623
  subnet_count        = length(var.control_plane_subnet_id_list)
  zone_count          = length(var.control_plane_subnet_zone_list)
}

############################################
# Control Plane nodes
############################################

resource "ibm_is_instance" "control_plane_node" {
  count = var.control_plane_count

  name           = "${local.prefix}-master-${count.index}"
  image          = var.vsi_image_id
  profile        = var.control_plane_instance_type
  resource_group = var.resource_group_id
  tags           = var.tags

  primary_network_interface {
    name            = "eth0"
    subnet          = var.control_plane_subnet_id_list[count.index % local.subnet_count]
    security_groups = var.control_plane_security_group_id_list
  }

  dedicated_host = length(var.control_plane_dedicated_host_id_list) > 0 ? var.control_plane_dedicated_host_id_list[count.index % local.zone_count] : null

  vpc  = var.vpc_id
  zone = var.control_plane_subnet_zone_list[count.index % local.zone_count]
  keys = []

  user_data = var.control_plane_ignition
}

############################################
# Machine Config LB Backend Pool Member
############################################

resource "ibm_is_lb_pool_member" "machine_config" {
  count = var.control_plane_count

  lb             = var.lb_kubernetes_api_private_id
  pool           = var.lb_pool_machine_config_id
  port           = local.port_machine_config
  target_address = ibm_is_instance.control_plane_node[count.index].primary_network_interface.0.primary_ipv4_address
}
