package protect

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
	"weka-deployment/common"

	"github.com/weka/go-cloud-lib/logging"
)

type RequestBody struct {
	Vm string `json:"vm"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	stateContainerName := os.Getenv("STATE_CONTAINER_NAME")
	stateStorageName := os.Getenv("STATE_STORAGE_NAME")
	prefix := os.Getenv("PREFIX")
	clusterName := os.Getenv("CLUSTER_NAME")

	outputs := make(map[string]interface{})
	resData := make(map[string]interface{})
	var invokeRequest common.InvokeRequest

	ctx := r.Context()
	logger := logging.LoggerFromCtx(ctx)

	d := json.NewDecoder(r.Body)
	err := d.Decode(&invokeRequest)
	if err != nil {
		logger.Error().Msg("Bad request")
		return
	}

	var reqData map[string]interface{}
	err = json.Unmarshal(invokeRequest.Data["req"], &reqData)
	if err != nil {
		logger.Error().Msg("Bad request")
		return
	}

	var data RequestBody

	if json.Unmarshal([]byte(reqData["Body"].(string)), &data) != nil {
		logger.Error().Msg("Bad request")
		return
	}

	vmScaleSetName := common.GetVmScaleSetName(prefix, clusterName)

	instanceName := strings.Split(data.Vm, ":")[0]
	hostName := strings.Split(data.Vm, ":")[1]
	instanceId := common.GetScaleSetVmIndex(instanceName)

	maxAttempts := 10
	authSleepInterval := time.Minute * 2

	err = common.RetrySetDeletionProtectionAndReport(ctx, subscriptionId, resourceGroupName, stateContainerName, stateStorageName, vmScaleSetName, instanceId, hostName, maxAttempts, authSleepInterval)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resData["body"] = err.Error()
	} else {
		resData["body"] = "protection was set successfully"
	}
	outputs["res"] = resData
	invokeResponse := common.InvokeResponse{Outputs: outputs, Logs: nil, ReturnValue: nil}

	responseJson, _ := json.Marshal(invokeResponse)

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJson)
}
