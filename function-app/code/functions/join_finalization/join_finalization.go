package join_finalization

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"weka-deployment/common"

	"github.com/weka/go-cloud-lib/logging"
)

type RequestBody struct {
	Name string `json:"name"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	resData := make(map[string]interface{})

	ctx := r.Context()
	logger := logging.LoggerFromCtx(ctx)

	var data RequestBody
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		logger.Error().Msg("Bad request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	prefix := os.Getenv("PREFIX")
	clusterName := os.Getenv("CLUSTER_NAME")
	vmScaleSetName := fmt.Sprintf("%s-%s-vmss", prefix, clusterName)

	err = common.SetDeletionProtection(ctx, subscriptionId, resourceGroupName, vmScaleSetName, common.GetScaleSetVmIndex(data.Name), true)
	if err != nil {
		resData["body"] = err.Error()
	} else {
		resData["body"] = "set protection successfully"
	}

	responseJson, _ := json.Marshal(resData)
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJson)
}
