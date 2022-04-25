locals {
  # Common locals
  prefix      = var.cluster_id
  zones_all   = distinct(concat(var.zones_master, var.zones_worker))
  cluster_vpc = var.preexisting_vpc ? var.cluster_vpc : ibm_is_vpc.vpc[0].id

  # LB locals
  port_kubernetes_api   = 6443
  port_machine_config   = 22623
  control_plane_subnets = var.preexisting_vpc ? data.ibm_is_subnet.control_plane[*] : ibm_is_subnet.control_plane[*]
  compute_subnets       = var.preexisting_vpc ? data.ibm_is_subnet.compute[*] : ibm_is_subnet.control_plane[*]

  # SG locals
  subnet_cidr_blocks = concat(local.control_plane_subnets[*].ipv4_cidr_block, local.compute_subnets[*].ipv4_cidr_block)
}

data "ibm_is_subnet" "control_plane" {
  count = var.preexisting_vpc ? length(var.control_plane_subnets) : 0

  identifier = var.control_plane_subnets[count.index]
}

data "ibm_is_subnet" "compute" {
  count = var.preexisting_vpc ? length(var.compute_subnets) : 0

  identifier = var.compute_subnets[count.index]
}
