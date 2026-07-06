package relay

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func supportsGeminiSyncImageModel(modelName string) bool {
	modelName = normalizeMappedModelName(modelName)
	return strings.HasPrefix(modelName, "imagen")
}

func resolveChannelMappedModelName(ch *model.Channel, modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if ch == nil || modelName == "" {
		return modelName
	}

	modelMapping := strings.TrimSpace(ch.GetModelMapping())
	if modelMapping == "" || modelMapping == "{}" {
		return modelName
	}

	modelMap := make(map[string]string)
	if err := common.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
		return modelName
	}

	currentModel := modelName
	visitedModels := map[string]bool{currentModel: true}
	for {
		mappedModel, exists := modelMap[currentModel]
		mappedModel = strings.TrimSpace(mappedModel)
		if !exists || mappedModel == "" {
			break
		}
		if visitedModels[mappedModel] {
			break
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}
	return currentModel
}

func normalizeMappedModelName(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	modelName = strings.TrimPrefix(modelName, "models/")
	if idx := strings.Index(modelName, ":"); idx >= 0 {
		modelName = modelName[:idx]
	}
	return modelName
}
