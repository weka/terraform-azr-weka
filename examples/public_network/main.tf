provider "azurerm" {
  subscription_id = var.subscription_id
  partner_id      = "f13589d1-f10d-4c3b-ae42-3b1a8337eaf1"
  features {
  }
}

module "create-network" {
  source            = "../../modules/create_networks"
  prefix            = var.prefix
  rg_name           = var.rg_name
  address_space     = var.address_space
  subnet_prefixes   = var.subnet_prefixes
  sg_ssh_range      = var.sg_ssh_range
}

module "deploy-weka" {
  source                = "../.."
  prefix                = var.prefix
  rg_name               = var.rg_name
  vnet_name             = module.create-network.vnet-name
  vnet_rg_name          = module.create-network.vnet_rg_name
  subnet_name           = module.create-network.subnets-name
  sg_id                 = module.create-network.sg-id
  get_weka_io_token     = var.get_weka_io_token
  cluster_name          = var.cluster_name
  subnet_delegation     = var.subnet_delegation
  set_obs_integration   = var.set_obs_integration
  instance_type         = var.instance_type
  cluster_size          = var.cluster_size
  tiering_ssd_percent   = var.tiering_ssd_percent
  subscription_id       = var.subscription_id
  private_dns_zone_name = module.create-network.private-dns-zone-name
  depends_on            = [module.create-network]
}
