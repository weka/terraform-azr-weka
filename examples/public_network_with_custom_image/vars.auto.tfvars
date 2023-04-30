prefix              = "weka"
rg_name             = "weka-rg"
address_space       = "10.0.0.0/16"
subnet_prefixes     = ["10.0.2.0/24","10.0.3.0/24","10.0.4.0/24","10.0.5.0/24"]
subnet_delegation   = "10.0.1.0/25"
cluster_name        = "poc"
instance_type       = "Standard_L8s_v3"
set_obs_integration = true
tiering_ssd_percent = 20
cluster_size        = 6
install_ofed        = false