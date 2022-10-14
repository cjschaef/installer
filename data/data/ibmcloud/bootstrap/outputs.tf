output "bootstrap_ip" {
  value = local.public_endpoints ? module.main.bootstrap_floating_ip : module.main.boostrap_ip
}
