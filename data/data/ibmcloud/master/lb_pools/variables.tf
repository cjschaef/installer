#######################################
# Master LB Pools module variables
#######################################

variable "control_plane_count" {
  type = string
}

variable "control_plane_ips" {
  type = list(string)
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
