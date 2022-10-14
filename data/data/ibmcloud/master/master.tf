############################################
# Main Master Module
############################################

module "main" {
  source = "./main"

  cluster_id        = var.cluster_id
  vpc_id            = var.vpc_id
  resource_group_id = var.resource_group_id
  vsi_image_id      = var.vsi_image_id
  tags              = local.tags

  control_plane_count                  = var.master_count
  control_plane_dedicated_host_id_list = var.control_plane_dedicated_host_id_list
  control_plane_ignition               = var.ignition_master
  control_plane_instance_type          = var.ibmcloud_master_instance_type
  control_plane_security_group_id_list = var.control_plane_security_group_id_list
  control_plane_subnet_id_list         = var.control_plane_subnet_id_list
  control_plane_subnet_zone_list       = var.control_plane_subnet_zone_list

  lb_kubernetes_api_private_id = var.lb_kubernetes_api_private_id
  lb_pool_machine_config_id    = var.lb_pool_machine_config_id
}


############################################
# LB Backend Pool Module
############################################
# Remaining LB backend pool members are configured separately
# due to timing windows for IBM Cloud ALB's in relation to
# https://issues.redhat.com/browse/OCPBUGS-1327

module "lb_pools" {
  source     = "./lb_pools"
  depends_on = [module.main]

  lb_kubernetes_api_public_id       = var.lb_kubernetes_api_public_id
  lb_pool_kubernetes_api_public_id  = var.lb_pool_kubernetes_api_public_id
  lb_kubernetes_api_private_id      = var.lb_kubernetes_api_private_id
  lb_pool_kubernetes_api_private_id = var.lb_pool_kubernetes_api_private_id

  control_plane_ips   = module.main.control_plane_ips
  control_plane_count = var.master_count

  public_endpoints = local.public_endpoints
}
