package resize

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"weka-deployment/common"

	"github.com/weka/go-cloud-lib/logging"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	resData := make(map[string]interface{})

	stateContainerName := os.Getenv("STATE_CONTAINER_NAME")
	stateStorageName := os.Getenv("STATE_STORAGE_NAME")
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroupName := os.Getenv("RESOURCE_GROUP_NAME")
	prefix := os.Getenv("PREFIX")
	clusterName := os.Getenv("CLUSTER_NAME")

	ctx := r.Context()
	logger := logging.LoggerFromCtx(ctx)

	var size struct {
		Value *int `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&size); err != nil {
		err = fmt.Errorf("cannot decode the request: %v", err)
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if size.Value == nil {
		err := fmt.Errorf("wrong request format. 'new_size' is required")
		logger.Error().Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Info().Msgf("The requested new size is %d", *size.Value)

	var err error

	minCusterSize := 6
	if *size.Value < minCusterSize {
		err = fmt.Errorf("invalid size, minimal cluster size is %d", minCusterSize)
		logger.Error().Err(err).Send()
	}

	if err == nil {
		vmScaleSetName := fmt.Sprintf("%s-%s-vmss", prefix, clusterName)
		err = updateDesiredClusterSize(ctx, *size.Value, subscriptionId, resourceGroupName, vmScaleSetName, stateContainerName, stateStorageName)
	}

	if err != nil {
		resData["body"] = err.Error()
	} else {
		resData["body"] = fmt.Sprintf("New cluster size: %d", *size.Value)
	}

	responseJson, _ := json.Marshal(resData)

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJson)
}

func updateDesiredClusterSize(ctx context.Context, newSize int, subscriptionId, resourceGroupName, vmScaleSetName, stateContainerName, stateStorageName string) error {
	state, err := common.ReadState(ctx, stateStorageName, stateContainerName)
	if err != nil {
		return err
	}

	if !state.Clusterized {
		err = fmt.Errorf("weka cluster is not ready")
		logger := logging.LoggerFromCtx(ctx)
		logger.Error().Err(err).Send()
		return err
	}
	oldSize := state.DesiredSize
	state.DesiredSize = newSize

	err = common.WriteState(ctx, stateStorageName, stateContainerName, state)
	if err != nil {
		err = fmt.Errorf("cannot update state to %d: %v", newSize, err)
		return err
	}

	if oldSize < newSize {
		err = common.UpdateVmScaleSetNum(ctx, subscriptionId, resourceGroupName, vmScaleSetName, int64(newSize))
		if err != nil {
			err = fmt.Errorf("cannot increase scale set %s capacity from %d to %d: %v", vmScaleSetName, oldSize, newSize, err)
			return err
		}
	}
	return nil
}
