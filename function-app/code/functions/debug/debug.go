package debug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"weka-deployment/common"
	"weka-deployment/functions/clusterize"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	stateContainerName := os.Getenv("STATE_CONTAINER_NAME")
	stateStorageName := os.Getenv("STATE_STORAGE_NAME")
	hostsNum := os.Getenv("HOSTS_NUM")
	clusterName := os.Getenv("CLUSTER_NAME")
	computeMemory := os.Getenv("COMPUTE_MEMORY")
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	setObs := os.Getenv("SET_OBS")
	obsName := os.Getenv("OBS_NAME")
	obsContainerName := os.Getenv("OBS_CONTAINER_NAME")
	obsAccessKey := os.Getenv("OBS_ACCESS_KEY")
	location := os.Getenv("LOCATION")
	drivesContainerNum := os.Getenv("NUM_DRIVE_CONTAINERS")
	computeContainerNum := os.Getenv("NUM_COMPUTE_CONTAINERS")
	frontendContainerNum := os.Getenv("NUM_FRONTEND_CONTAINERS")
	tieringSsdPercent := os.Getenv("TIERING_SSD_PERCENT")
	prefix := os.Getenv("PREFIX")
	keyVaultUri := os.Getenv("KEY_VAULT_URI")
	// data protection-related vars
	stripeWidth, _ := strconv.Atoi(os.Getenv("STRIPE_WIDTH"))
	protectionLevel, _ := strconv.Atoi(os.Getenv("PROTECTION_LEVEL"))
	hotspare, _ := strconv.Atoi(os.Getenv("HOTSPARE"))

	vmScaleSetName := fmt.Sprintf("%s-%s-vmss", prefix, clusterName)

	outputs := make(map[string]interface{})
	resData := make(map[string]interface{})

	ctx := r.Context()
	logger := common.LoggerFromCtx(ctx)

	var invokeRequest common.InvokeRequest

	var function struct {
		Function *string `json:"function"`
		IpIndex  *string `json:"ip_index"`
	}

	if err := json.NewDecoder(r.Body).Decode(&invokeRequest); err != nil {
		err = fmt.Errorf("cannot decode the request: %v", err)
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var reqData map[string]interface{}
	err := json.Unmarshal(invokeRequest.Data["req"], &reqData)
	if err != nil {
		err = fmt.Errorf("cannot unmarshal the request data: %v", err)
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if json.Unmarshal([]byte(reqData["Body"].(string)), &function) != nil {
		err = fmt.Errorf("cannot unmarshal the request body: %v", err)
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if function.Function == nil {
		err := fmt.Errorf("wrong request format. 'function' is required")
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Info().Msgf("The requested function is %s", *function.Function)
	var result interface{}

	if *function.Function == "clusterize" {
		state, err := common.ReadState(ctx, stateStorageName, stateContainerName)
		if err != nil {
			result = clusterize.GetErrorScript(err)
		} else {
			params := clusterize.ClusterizationParams{
				SubscriptionId:     subscriptionId,
				ResourceGroupName:  resourceGroupName,
				Location:           location,
				Prefix:             prefix,
				KeyVaultUri:        keyVaultUri,
				StateContainerName: stateContainerName,
				StateStorageName:   stateStorageName,
				Cluster: clusterize.WekaClusterParams{
					HostsNum:             hostsNum,
					Name:                 clusterName,
					ComputeMemory:        computeMemory,
					DrivesContainerNum:   drivesContainerNum,
					ComputeContainerNum:  computeContainerNum,
					FrontendContainerNum: frontendContainerNum,
					TieringSsdPercent:    tieringSsdPercent,
					DataProtection: clusterize.DataProtectionParams{
						StripeWidth:     stripeWidth,
						ProtectionLevel: protectionLevel,
						Hotspare:        hotspare,
					},
				},
				Obs: clusterize.ObsParams{
					SetObs:        setObs,
					Name:          obsName,
					ContainerName: obsContainerName,
					AccessKey:     obsAccessKey,
				},
			}
			result = clusterize.HandleLastClusterVm(ctx, state, params)
		}
	} else if *function.Function == "instances" {
		expand := "instanceView"
		instances, err1 := common.GetScaleSetInstances(ctx, subscriptionId, resourceGroupName, vmScaleSetName, &expand)
		if err1 != nil {
			result = err1.Error()
		} else {
			result = instances
		}
	} else if *function.Function == "interfaces" {
		interfaces, err1 := common.GetScaleSetVmsNetworkInterfaces(ctx, subscriptionId, resourceGroupName, vmScaleSetName)
		if err1 != nil {
			result = err1.Error()
		} else {
			result = interfaces
		}
	} else if *function.Function == "ip" {
		if function.Function == nil {
			err := fmt.Errorf("wrong request format. 'ip_index' is required for fucntion 'ip'")
			logger.Error().Err(err).Send()
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ips, err1 := common.GetPublicIp(ctx, subscriptionId, resourceGroupName, vmScaleSetName, prefix, clusterName, *function.IpIndex)
		if err1 != nil {
			result = err1.Error()
		} else {
			result = ips
		}
	} else {
		result = "unsupported function"
	}

	resData["body"] = result
	outputs["res"] = resData
	invokeResponse := common.InvokeResponse{Outputs: outputs, Logs: nil, ReturnValue: nil}

	responseJson, _ := json.Marshal(invokeResponse)

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJson)
}
