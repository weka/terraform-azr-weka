package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"weka-deployment/lib/types"
	"weka-deployment/protocol"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type InvokeRequest struct {
	Data     map[string]json.RawMessage
	Metadata map[string]interface{}
}

type InvokeResponse struct {
	Outputs     map[string]interface{}
	Logs        []string
	ReturnValue interface{}
}

type ClusterState struct {
	InitialSize int      `json:"initial_size"`
	DesiredSize int      `json:"desired_size"`
	Instances   []string `json:"instances"`
	Clusterized bool     `json:"clusterized"`
}

func leaseContainer(subscriptionId, resourceGroupName, storageAccountName, containerName string, leaseIdIn *string, action armstorage.LeaseContainerRequestAction) (leaseIdOut *string, err error) {
	ctx := context.Background()

	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Msgf("azidentity.NewDefaultAzureCredential: %s", err)
		return
	}

	containerClient, err := armstorage.NewBlobContainersClient(subscriptionId, credential, nil)
	duration := int32(60)
	for i := 1; i < 1000; i++ {
		lease, err2 := containerClient.Lease(ctx, resourceGroupName, storageAccountName, containerName,
			&armstorage.BlobContainersClientLeaseOptions{
				Parameters: &armstorage.LeaseContainerRequest{
					Action:        &action,
					LeaseDuration: &duration,
					LeaseID:       leaseIdIn,
				},
			})
		err = err2
		if err != nil {
			if leaseErr, ok := err.(*azcore.ResponseError); ok && leaseErr.ErrorCode == "ContainerOperationFailure" {
				buf := new(bytes.Buffer)
				buf.ReadFrom(leaseErr.RawResponse.Body)
				if !strings.Contains(buf.String(), "LeaseAlreadyPresent") {
					log.Error().Err(err).Send()
					return
				}
				log.Debug().Msg("lease in use, will retry in 1 sec")
				time.Sleep(time.Second)
			} else {
				log.Error().Err(err).Send()
				return
			}
		} else {
			leaseIdOut = lease.LeaseID
			return
		}
	}

	log.Error().Err(err).Send()
	return
}

func LockContainer(subscriptionId, resourceGroupName, storageAccountName, containerName string) (*string, error) {
	log.Debug().Msgf("locking %s", containerName)
	return leaseContainer(subscriptionId, resourceGroupName, storageAccountName, containerName, nil, armstorage.LeaseContainerRequestActionAcquire)
}

func UnlockContainer(subscriptionId, resourceGroupName, storageAccountName, containerName string, leaseId *string) (*string, error) {
	log.Debug().Msgf("unlocking %s", containerName)
	return leaseContainer(subscriptionId, resourceGroupName, storageAccountName, containerName, leaseId, armstorage.LeaseContainerRequestActionRelease)
}

func ReadBlobObject(stateStorageName, containerName, blobName string) (state []byte, err error) {
	ctx := context.Background()

	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Msgf("azidentity.NewDefaultAzureCredential: %s", err)
		return
	}

	blobClient, err := azblob.NewClient(getBlobUrl(stateStorageName), credential, nil)
	if err != nil {
		log.Error().Msgf("azblob.NewClient: %s", err)
		return
	}

	downloadResponse, err := blobClient.DownloadStream(ctx, containerName, blobName, nil)
	if err != nil {
		log.Error().Msgf("blobClient.DownloadStream: %s", err)
		return
	}

	state, err = io.ReadAll(downloadResponse.Body)
	if err != nil {
		log.Error().Err(err).Send()
	}

	return

}

func ReadState(stateStorageName, containerName string) (state ClusterState, err error) {
	stateAsByteArray, err := ReadBlobObject(stateStorageName, containerName, "state")
	if err != nil {
		return
	}
	err = json.Unmarshal(stateAsByteArray, &state)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	return
}

func WriteBlobObject(stateStorageName, containerName, blobName string, state []byte) (err error) {
	ctx := context.Background()

	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	blobClient, err := azblob.NewClient(getBlobUrl(stateStorageName), credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	_, err = blobClient.UploadBuffer(ctx, containerName, blobName, state, &azblob.UploadBufferOptions{})

	return

}

func WriteState(stateStorageName, containerName string, state ClusterState) (err error) {
	stateAsByteArray, err := json.Marshal(state)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	err = WriteBlobObject(stateStorageName, containerName, "state", stateAsByteArray)
	return
}

func getBlobUrl(storageName string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/", storageName)
}

type ShutdownRequired struct {
	Message string
}

func (e *ShutdownRequired) Error() string {
	return e.Message
}

func AddInstanceToState(subscriptionId, resourceGroupName, stateStorageName, stateContainerName, newInstance string) (state ClusterState, err error) {
	leaseId, err := LockContainer(subscriptionId, resourceGroupName, stateStorageName, stateContainerName)
	if err != nil {
		return
	}

	state, err = ReadState(stateStorageName, stateContainerName)
	if err != nil {
		return
	}

	if len(state.Instances) >= state.InitialSize {
		message := "cluster size is already satisfied"
		err = &ShutdownRequired{
			Message: message,
		}
		log.Error().Err(err).Send()
	} else if state.Clusterized {
		err = &ShutdownRequired{
			Message: "cluster is already clusterized",
		}
		log.Error().Err(err).Send()
	} else {
		state.Instances = append(state.Instances, newInstance)
		err = WriteState(stateStorageName, stateContainerName, state)
	}
	_, err2 := UnlockContainer(subscriptionId, resourceGroupName, stateStorageName, stateContainerName, leaseId)
	if err2 != nil {
		if err == nil {
			err = err2
		}
		log.Error().Msgf("unlocking %s failed", stateStorageName)
	}
	return
}

func UpdateClusterized(subscriptionId, resourceGroupName, stateStorageName, stateContainerName string) (state ClusterState, err error) {
	leaseId, err := LockContainer(subscriptionId, resourceGroupName, stateStorageName, stateContainerName)
	if err != nil {
		return
	}

	state, err = ReadState(stateStorageName, stateContainerName)
	if err != nil {
		return
	}

	state.Instances = []string{}
	state.Clusterized = true
	err = WriteState(stateStorageName, stateContainerName, state)
	_, err2 := UnlockContainer(subscriptionId, resourceGroupName, stateStorageName, stateContainerName, leaseId)
	if err2 != nil {
		if err == nil {
			err = err2
		}
		log.Error().Msgf("unlocking %s failed", stateStorageName)
	}
	return
}

func CreateStorageAccount(subscriptionId, resourceGroupName, obsName, location string) (accessKey string, err error) {
	log.Info().Msgf("creating storage account: %s", obsName)
	ctx := context.Background()
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	client, err := armstorage.NewAccountsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	skuName := armstorage.SKUNameStandardZRS
	kind := armstorage.KindStorageV2
	_, err = client.BeginCreate(ctx, resourceGroupName, obsName, armstorage.AccountCreateParameters{
		Kind:     &kind,
		Location: &location,
		SKU: &armstorage.SKU{
			Name: &skuName,
		},
	}, nil)

	if err != nil {
		if azerr, ok := err.(*azcore.ResponseError); ok {
			if azerr.ErrorCode == "StorageAccountAlreadyExists" {
				log.Debug().Msgf("storage account %s already exists", obsName)
				err = nil
			} else {
				log.Error().Msgf("storage creation failed: %s", err)
				return
			}
		} else {
			log.Error().Msgf("storage creation failed: %s", err)
			return
		}
	}

	for i := 0; i < 10; i++ {
		accessKey, err = getStorageAccountAccessKey(subscriptionId, resourceGroupName, obsName)

		if err != nil {
			if azerr, ok := err.(*azcore.ResponseError); ok {
				if azerr.ErrorCode == "StorageAccountIsNotProvisioned" {
					log.Debug().Msgf("new storage account is not ready will retry in 1M")
					time.Sleep(time.Minute)
				} else {
					log.Error().Err(err).Send()
					return
				}
			} else {
				log.Error().Err(err).Send()
				return
			}
		} else {
			log.Debug().Msgf("storage account '%s' is ready for use", obsName)
			break
		}
	}

	return
}

func getStorageAccountAccessKey(subscriptionId, resourceGroupName, obsName string) (accessKey string, err error) {
	ctx := context.Background()
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	client, err := armstorage.NewAccountsClient(subscriptionId, credential, nil)
	keys, err := client.ListKeys(ctx, resourceGroupName, obsName, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	accessKey = *keys.Keys[0].Value
	return
}

func CreateContainer(storageAccountName, containerName string) (err error) {
	log.Info().Msgf("creating obs container %s in storage account %s", containerName, storageAccountName)
	ctx := context.Background()

	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	blobClient, err := azblob.NewClient(getBlobUrl(storageAccountName), credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	_, err = blobClient.CreateContainer(ctx, containerName, nil)
	if err != nil {
		if azerr, ok := err.(*azcore.ResponseError); ok {
			if azerr.ErrorCode == "ContainerAlreadyExists" {
				log.Info().Msgf("obs container %s already exists", containerName)
				err = nil
				return
			}
		}
		log.Error().Msgf("obs container creation failed: %s", err)
	}
	return
}

func GetKeyVaultValue(keyVaultUri, secretName string) (secret string, err error) {
	log.Info().Msgf("fetching key vault secret: %s", secretName)
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	ctx := context.Background()
	client, err := azsecrets.NewClient(keyVaultUri, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	resp, err := client.GetSecret(ctx, secretName, "", nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	secret = *resp.Value

	return
}

// Gets all network interfaces in a VM scale set
// see https://learn.microsoft.com/en-us/rest/api/virtualnetwork/network-interface-in-vm-ss/list-virtual-machine-scale-set-network-interfaces
func GetScaleSetVmsNetworkInterfaces(subscriptionId, resourceGroupName, vmScaleSetName string) (networkInterfaces []*armnetwork.Interface, err error) {
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	ctx := context.Background()
	client, err := armnetwork.NewInterfacesClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	pager := client.NewListVirtualMachineScaleSetNetworkInterfacesPager(resourceGroupName, vmScaleSetName, nil)

	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			log.Error().Err(err).Send()
			return nil, err
		}
		networkInterfaces = append(networkInterfaces, nextResult.Value...)
	}
	return
}

func GetVmsPrivateIps(subscriptionId, resourceGroupName, vmScaleSetName string) (vmsPrivateIps map[string]string, err error) {
	log.Info().Msg("fetching scale set vms private ips")

	networkInterfaces, err := GetScaleSetVmsNetworkInterfaces(subscriptionId, resourceGroupName, vmScaleSetName)
	if err != nil {
		return
	}

	vmsPrivateIps = make(map[string]string)
	for _, networkInterface := range networkInterfaces {
		vmNameParts := strings.Split(*networkInterface.Properties.VirtualMachine.ID, "/")
		vmNamePartsLen := len(vmNameParts)
		vmName := fmt.Sprintf("%s_%s", vmNameParts[vmNamePartsLen-3], vmNameParts[vmNamePartsLen-1])
		vmsPrivateIps[vmName] = *networkInterface.Properties.IPConfigurations[0].Properties.PrivateIPAddress
	}
	return
}

func UpdateVmScaleSetNum(subscriptionId, resourceGroupName, vmScaleSetName string, newSize int64) (err error) {
	log.Info().Msg("updating scale set vms num")
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	client, err := armcompute.NewVirtualMachineScaleSetsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	ctx := context.Background()
	_, err = client.BeginUpdate(ctx, resourceGroupName, vmScaleSetName, armcompute.VirtualMachineScaleSetUpdate{
		SKU: &armcompute.SKU{
			Capacity: &newSize,
		},
	}, nil)
	if err != nil {
		log.Error().Err(err).Send()
	}
	return
}

func GetRoleDefinitionByRoleName(ctx context.Context, roleName, scope string) (*armauthorization.RoleDefinition, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	client, err := armauthorization.NewRoleDefinitionsClient(cred, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	var results []*armauthorization.RoleDefinition
	filter := fmt.Sprintf("roleName eq '%s'", roleName)

	pager := client.NewListPager("/", &armauthorization.RoleDefinitionsClientListOptions{Filter: &filter})
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			log.Error().Err(err).Send()
			return nil, err
		}
		results = append(results, nextResult.Value...)
	}

	// filter the needed role out of all built-in ones
	var roleDefs []*armauthorization.RoleDefinition
	for _, res := range results {
		if *res.Properties.RoleName == roleName {
			roleDefs = append(roleDefs, res)
		}
	}

	if len(roleDefs) < 1 {
		err := fmt.Errorf("cannot find az role definition with name '%s'", roleName)
		log.Error().Err(err).Send()
		return nil, err
	}
	if len(roleDefs) > 1 {
		err := fmt.Errorf("found several az role definitions with name '%s', check the name", roleName)
		log.Error().Err(err).Send()
		return nil, err
	}
	return roleDefs[0], nil
}

func AssignStorageBlobDataContributorRoleToScaleSet(
	ctx context.Context, subscriptionId, resourceGroupName, vmScaleSetName, storageAccountName, containerName string,
) (*armauthorization.RoleAssignment, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionId, cred, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	scaleSet, err := getScaleSet(subscriptionId, resourceGroupName, vmScaleSetName)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	scope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/%s",
		subscriptionId,
		resourceGroupName,
		storageAccountName,
		containerName,
	)

	roleDefinition, err := GetRoleDefinitionByRoleName(ctx, "Storage Blob Data Contributor", scope)
	if err != nil {
		err = fmt.Errorf("cannot get the role definition: %v", err)
		log.Error().Err(err).Send()
		return nil, err
	}

	// see https://learn.microsoft.com/en-us/rest/api/authorization/role-assignments/create
	res, err := client.Create(
		ctx,
		scope,
		uuid.New().String(), // az docs say it should be GUID
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				RoleDefinitionID: roleDefinition.ID,
				PrincipalID:      scaleSet.Identity.PrincipalID,
			},
		},
		nil,
	)
	if err != nil {
		err = fmt.Errorf("cannot create the role assignment: %v", err)
		log.Error().Err(err).Send()
		return nil, err
	}

	return &res.RoleAssignment, nil
}

type ScaleSetInfo struct {
	Id            string
	Name          string
	AdminUsername string
	AdminPassword string
	Capacity      int
	VMSize        string
}

func getScaleSet(subscriptionId, resourceGroupName, vmScaleSetName string) (*armcompute.VirtualMachineScaleSet, error) {
	ctx := context.Background()
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	client, err := armcompute.NewVirtualMachineScaleSetsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	scaleSet, err := client.Get(ctx, resourceGroupName, vmScaleSetName, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}
	return &scaleSet.VirtualMachineScaleSet, nil
}

// Gets single scale set info
// see https://learn.microsoft.com/en-us/rest/api/compute/virtual-machine-scale-sets/get
func GetScaleSetInfo(subscriptionId, resourceGroupName, vmScaleSetName, keyVaultUri string) (*ScaleSetInfo, error) {
	scaleSet, err := getScaleSet(subscriptionId, resourceGroupName, vmScaleSetName)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	wekaPassword, err := GetWekaClusterPassword(keyVaultUri)
	if err != nil {
		log.Error().Err(err).Send()
		return nil, err
	}

	scaleSetInfo := ScaleSetInfo{
		Id:            *scaleSet.ID,
		Name:          *scaleSet.Name,
		AdminUsername: "admin",
		AdminPassword: wekaPassword,
		Capacity:      int(*scaleSet.SKU.Capacity),
		VMSize:        *scaleSet.SKU.Name,
	}
	return &scaleSetInfo, err
}

// Gets a list of all VMs in a scale set
// see https://learn.microsoft.com/en-us/rest/api/compute/virtual-machine-scale-set-vms/list
func GetScaleSetInstances(subscriptionId, resourceGroupName, vmScaleSetName string, expand *string) (vms []*armcompute.VirtualMachineScaleSetVM, err error) {
	ctx := context.Background()
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	client, err := armcompute.NewVirtualMachineScaleSetVMsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	pager := client.NewListPager(
		resourceGroupName, vmScaleSetName, &armcompute.VirtualMachineScaleSetVMsClientListOptions{
			Expand: expand,
		})

	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			err = fmt.Errorf("failed to advance page getting images list: %v", err)
			log.Error().Err(err).Send()
			return nil, err
		}
		vms = append(vms, nextResult.Value...)
	}
	return
}

type ScaleSetInstanceInfo struct {
	Id        string
	PrivateIp string
}

func GetScaleSetVmId(resourceId string) string {
	vmNameParts := strings.Split(resourceId, "/")
	vmNamePartsLen := len(vmNameParts)
	vmId := vmNameParts[vmNamePartsLen-1]
	return vmId
}

func GetScaleSetInstancesInfo(subscriptionId, resourceGroupName, vmScaleSetName string) (instances []ScaleSetInstanceInfo, err error) {
	netInterfaces, err := GetScaleSetVmsNetworkInterfaces(subscriptionId, resourceGroupName, vmScaleSetName)
	if err != nil {
		return
	}
	instanceIdPrivateIp := map[string]string{}

	for _, ni := range netInterfaces {
		id := GetScaleSetVmId(*ni.Properties.VirtualMachine.ID)
		privateIp := *ni.Properties.IPConfigurations[0].Properties.PrivateIPAddress
		instanceIdPrivateIp[id] = privateIp
	}

	vms, err := GetScaleSetInstances(subscriptionId, resourceGroupName, vmScaleSetName, nil)
	if err != nil {
		return
	}
	for _, vm := range vms {
		id := GetScaleSetVmId(*vm.ID)
		// get private ip if exists
		var privateIp string
		if val, ok := instanceIdPrivateIp[id]; ok {
			privateIp = val
		}
		instanceInfo := ScaleSetInstanceInfo{
			Id:        id,
			PrivateIp: privateIp,
		}
		instances = append(instances, instanceInfo)
	}
	return
}

func SetDeletionProtection(subscriptionId, resourceGroupName, vmScaleSetName, instanceNameOrId string, protect bool) (err error) {
	instanceNameParts := strings.Split(instanceNameOrId, "_")
	instanceId := instanceNameParts[len(instanceNameParts)-1]
	log.Info().Msgf("Setting deletion protection: %t on instanceId %s", protect, instanceId)

	ctx := context.Background()
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	client, err := armcompute.NewVirtualMachineScaleSetVMsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	_, err = client.BeginUpdate(
		ctx,
		resourceGroupName,
		vmScaleSetName,
		instanceId,
		armcompute.VirtualMachineScaleSetVM{
			Properties: &armcompute.VirtualMachineScaleSetVMProperties{
				ProtectionPolicy: &armcompute.VirtualMachineScaleSetVMProtectionPolicy{
					ProtectFromScaleSetActions: &protect,
				},
			},
		},
		nil)

	return
}

func GetWekaClusterPassword(keyVaultUri string) (password string, err error) {
	return GetKeyVaultValue(keyVaultUri, "weka-password")
}

func GetVmScaleSetName(prefix, clusterName string) string {
	return fmt.Sprintf("%s-%s-vmss", prefix, clusterName)
}

func GetScaleSetInstanceIds(subscriptionId, resourceGroupName, vmScaleSetName string) (instanceIds []string, err error) {
	vms, err := GetScaleSetInstances(subscriptionId, resourceGroupName, vmScaleSetName, nil)
	if err != nil {
		return
	}

	for _, vm := range vms {
		instanceIds = append(instanceIds, GetScaleSetVmId(*vm.ID))
	}

	return
}

type InstanceIdsSet map[string]types.Nilt

func GetInstanceIdsSet(scaleResponse protocol.ScaleResponse) InstanceIdsSet {
	instanceIdsSet := make(InstanceIdsSet)
	for _, instance := range scaleResponse.Hosts {
		instanceIdsSet[instance.InstanceId] = types.Nilv
	}
	return instanceIdsSet
}

func GetSpecificScaleSetInstances(subscriptionId, resourceGroupName, vmScaleSetName string, instanceIds []string) (vms []*armcompute.VirtualMachineScaleSetVM, err error) {
	allVms, err := GetScaleSetInstances(subscriptionId, resourceGroupName, vmScaleSetName, nil)
	if err != nil {
		return
	}

	instanceIdsSet := make(InstanceIdsSet)
	for _, instanceId := range instanceIds {
		instanceIdsSet[instanceId] = types.Nilv
	}

	for _, vm := range allVms {
		if _, ok := instanceIdsSet[GetScaleSetVmId(*vm.ID)]; !ok {
			vms = append(vms, vm)
		}
	}

	return
}

func TerminateSclaeSetInstances(subscriptionId, resourceGroupName, vmScaleSetName string, terminateInstanceIds []string) (terminatedInstances []string, errs []error) {
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	client, err := armcompute.NewVirtualMachineScaleSetVMsClient(subscriptionId, credential, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	ctx := context.Background()

	if len(terminateInstanceIds) == 0 {
		return
	}
	force := true
	for _, instanceId := range terminateInstanceIds {
		err = SetDeletionProtection(subscriptionId, resourceGroupName, vmScaleSetName, instanceId, false)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = client.BeginDelete(ctx, resourceGroupName, vmScaleSetName, instanceId, &armcompute.VirtualMachineScaleSetVMsClientBeginDeleteOptions{
			ForceDeletion: &force,
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		terminatedInstances = append(terminatedInstances, instanceId)
	}

	return
}
