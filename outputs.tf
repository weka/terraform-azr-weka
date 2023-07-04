locals {
  vm_ips            = var.private_network ? "az vmss nic list -g ${var.rg_name} --vmss-name ${azurerm_linux_virtual_machine_scale_set.vmss.name} --subscription ${var.subscription_id} --query \"[].ipConfigurations[]\" | jq -r '.[] | select(.name==\"ipconfig0\")'.privateIPAddress" : "az vmss list-instance-public-ips -g ${var.rg_name} --name ${azurerm_linux_virtual_machine_scale_set.vmss.name} --subscription ${var.subscription_id} --query \"[].ipAddress\" \n"
  clients_ips       = var.clients_number > 0 ? var.private_network ? "az vmss nic list -g ${var.rg_name} --vmss-name ${module.clients[0].client-name} --subscription ${var.subscription_id} --query \"[].ipConfigurations[]\" | jq -r '.[] | select(.name==\"ipconfig0\")'.privateIPAddress" : "az vmss list-instance-public-ips -g ${var.rg_name} --name ${module.clients[0].client-name} --subscription ${var.subscription_id} --query \"[].ipAddress\" \n" : ""
  ssh_keys_commands = "########################################## Download ssh keys command from blob ###########################################################\n az keyvault secret download --file private.pem --encoding utf-8 --vault-name  ${azurerm_key_vault.key_vault.name} --name private-key --query \"value\" \n az keyvault secret download --file public.pub --encoding utf-8 --vault-name  ${azurerm_key_vault.key_vault.name} --name public-key --query \"value\"\n"
  blob_commands     = var.ssh_public_key == null ? local.ssh_keys_commands : ""
  path_ssh_keys     = var.ssh_public_key == null ? "${local.ssh_path}-public-key.pub \n ${local.ssh_path}-private-key.pem" : "${var.ssh_private_key} \n ${var.ssh_public_key}"
}
output "cluster_helpers_commands" {
  value = <<EOT
########################################## Get clusterization status #####################################################################
function_key=$(az functionapp keys list --name ${azurerm_linux_function_app.function_app.name} --resource-group ${data.azurerm_resource_group.rg.name} --subscription ${var.subscription_id} --query functionKeys -o tsv)
curl --fail https://${var.prefix}-${var.cluster_name}-function-app.azurewebsites.net/api/status?code=$function_key -H "Content-Type:application/json" -d '{"type": "progress"}'

########################################## Get cluster status ############################################################################
function_key=$(az functionapp keys list --name ${azurerm_linux_function_app.function_app.name} --resource-group ${data.azurerm_resource_group.rg.name} --subscription ${var.subscription_id} --query functionKeys -o tsv)
curl --fail https://${var.prefix}-${var.cluster_name}-function-app.azurewebsites.net/api/status?code=$function_key

######################################### Fetch weka cluster password ####################################################################
az keyvault secret show --vault-name ${azurerm_key_vault.key_vault.name} --name weka-password | jq .value

${local.blob_commands}
############################################## Path to ssh keys  ##########################################################################
${local.path_ssh_keys}

################################################ Vms ips ##################################################################################
${local.vm_ips}
username: ${var.vm_username}

########################################## Resize cluster #################################################################################
function_key=$(az functionapp keys list --name ${azurerm_linux_function_app.function_app.name} --resource-group ${data.azurerm_resource_group.rg.name} --subscription ${var.subscription_id} --query functionKeys -o tsv)
curl --fail https://${var.prefix}-${var.cluster_name}-function-app.azurewebsites.net/api/resize?code=$function_key -H "Content-Type:application/json" -d '{"value":ENTER_NEW_VALUE_HERE}'

########################################## Clients ips #################################################################################
${local.clients_ips}
username: ${var.vm_username}
EOT
  description = "Useful commands and script to interact with weka cluster"
}
