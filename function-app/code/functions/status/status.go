package status

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"
	"weka-deployment/common"

	"github.com/weka/go-cloud-lib/connectors"
	"github.com/weka/go-cloud-lib/lib/jrpc"
	"github.com/weka/go-cloud-lib/lib/weka"
	"github.com/weka/go-cloud-lib/logging"
	"github.com/weka/go-cloud-lib/protocol"
)

func GetReports(ctx context.Context, stateStorageName, stateContainerName string) (reports protocol.Reports, err error) {
	logger := logging.LoggerFromCtx(ctx)
	logger.Info().Msg("fetching cluster status...")

	state, err := common.ReadState(ctx, stateStorageName, stateContainerName)
	if err != nil {
		return
	}
	reports.ReadyForClusterization = state.Instances
	reports.Progress = state.Progress
	reports.Errors = state.Errors
	reports.Debug = state.Debug
	reports.InProgress = state.InProgress
	reports.Summary = state.Summary
	reports.InProgress = state.InProgress

	return
}

func GetClusterStatus(
	ctx context.Context, subscriptionId, resourceGroupName, vmScaleSetName, stateStorageName, stateContainerName, keyVaultUri string, refreshVmScaleSetName *string,
) (clusterStatus protocol.ClusterStatus, err error) {
	logger := logging.LoggerFromCtx(ctx)
	logger.Info().Msg("fetching cluster status...")

	state, err := common.ReadState(ctx, stateStorageName, stateContainerName)
	if err != nil {
		return
	}
	clusterStatus.InitialSize = state.InitialSize
	clusterStatus.DesiredSize = state.DesiredSize
	clusterStatus.Clusterized = state.Clusterized
	if !state.Clusterized {
		return
	}

	wekaPassword, err := common.GetWekaClusterPassword(ctx, keyVaultUri)
	if err != nil {
		return
	}

	jrpcBuilder := func(ip string) *jrpc.BaseClient {
		return connectors.NewJrpcClient(ctx, ip, weka.ManagementJrpcPort, "admin", wekaPassword)
	}

	vmIps, err := common.GetVmsPrivateIps(ctx, subscriptionId, resourceGroupName, vmScaleSetName)
	if err != nil {
		return
	}

	refreshVmIps := make(map[string]string, 0)
	if refreshVmScaleSetName != nil {
		refreshVmIps, err = common.GetVmsPrivateIps(ctx, subscriptionId, resourceGroupName, *refreshVmScaleSetName)
		if err != nil {
			return
		}
	}

	ips := make([]string, 0, len(vmIps)+len(refreshVmIps))
	for _, ip := range vmIps {
		ips = append(ips, ip)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(ips), func(i, j int) { ips[i], ips[j] = ips[j], ips[i] })
	logger.Info().Msgf("ips: %s", ips)
	jpool := &jrpc.Pool{
		Ips:     ips,
		Clients: map[string]*jrpc.BaseClient{},
		Active:  "",
		Builder: jrpcBuilder,
		Ctx:     ctx,
	}

	var rawWekaStatus json.RawMessage

	err = jpool.Call(weka.JrpcStatus, struct{}{}, &rawWekaStatus)
	if err != nil {
		return
	}

	wekaStatus := protocol.WekaStatus{}
	if err = json.Unmarshal(rawWekaStatus, &wekaStatus); err != nil {
		return
	}
	clusterStatus.WekaStatus = wekaStatus

	return
}

func Handler(w http.ResponseWriter, r *http.Request) {
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	stateContainerName := os.Getenv("STATE_CONTAINER_NAME")
	stateStorageName := os.Getenv("STATE_STORAGE_NAME")
	vmssStateStorageName := os.Getenv("VMSS_STATE_STORAGE_NAME")
	prefix := os.Getenv("PREFIX")
	clusterName := os.Getenv("CLUSTER_NAME")
	keyVaultUri := os.Getenv("KEY_VAULT_URI")

	ctx := r.Context()
	logger := logging.LoggerFromCtx(ctx)

	var invokeRequest common.InvokeRequest

	var requestBody struct {
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&invokeRequest); err != nil {
		err = fmt.Errorf("cannot decode the request: %v", err)
		logger.Error().Err(err).Send()
		common.WriteErrorResponse(w, err)
		return
	}

	var reqData map[string]interface{}
	err := json.Unmarshal(invokeRequest.Data["req"], &reqData)
	if err != nil {
		err = fmt.Errorf("cannot unmarshal the request data: %v", err)
		logger.Error().Err(err).Send()
		common.WriteErrorResponse(w, err)
		return
	}

	if reqData["Body"] != nil {
		if json.Unmarshal([]byte(reqData["Body"].(string)), &requestBody) != nil {
			err = fmt.Errorf("cannot unmarshal the request body: %v", err)
			logger.Error().Err(err).Send()
			common.WriteErrorResponse(w, err)
			return
		}
	}

	vmssState, err := common.ReadVmssState(ctx, vmssStateStorageName, stateContainerName)
	if err != nil {
		err = fmt.Errorf("cannot read vmss state to read get vmss version: %v", err)
		logger.Error().Err(err).Send()
		common.WriteErrorResponse(w, err)
		return
	}

	vmScaleSetName := common.GetVmScaleSetName(prefix, clusterName, vmssState.VmssVersion)
	var refreshVmssName *string
	if vmssState.RefreshStatus != common.RefreshNone {
		n := common.GetRefreshVmssName(vmScaleSetName, vmssState.VmssVersion)
		refreshVmssName = &n
	}

	var result interface{}
	if requestBody.Type == "" || requestBody.Type == "status" {
		result, err = GetClusterStatus(ctx, subscriptionId, resourceGroupName, vmScaleSetName, stateStorageName, stateContainerName, keyVaultUri, refreshVmssName)
	} else if requestBody.Type == "progress" {
		result, err = GetReports(ctx, stateStorageName, stateContainerName)
	} else if requestBody.Type == "vmss" {
		refreshVmssName := common.GetRefreshVmssName(vmScaleSetName, vmssState.VmssVersion)
		if vmssState.RefreshStatus == common.RefreshNone {
			refreshVmssName = ""
		}
		result = common.VMSSStateVerbose{
			VmssCreated:     vmssState.VmssCreated,
			VmssName:        vmScaleSetName,
			RefreshStatus:   vmssState.RefreshStatus.String(),
			RefreshVmssName: refreshVmssName,
			CurrentConfig:   vmssState.CurrentConfig,
		}
	} else {
		result = "Invalid status type"
	}

	if err != nil {
		common.WriteErrorResponse(w, err)
		return
	}
	common.WriteSuccessResponse(w, result)
}
