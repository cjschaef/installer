#######################################
# LB Pools Bootstrap module variables
#######################################

variable "bootstrap_node_ip" {
  type = string
}

variable "lb_kubernetes_api_private_id" {
  type = string
}

variable "lb_kubernetes_api_public_id" {
  type = string
}

variable "lb_pool_kubernetes_api_private_id" {
  type = string
}

variable "lb_pool_kubernetes_api_public_id" {
  type = string
}

variable "public_endpoints" {
  type = bool
}
