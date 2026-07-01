package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestChannelAbilityModelsIncludeModelMappingSources(t *testing.T) {
	modelMapping := `{
		"public-nano": "nanobanana",
		" public-gpt-image ": " gpt-image-2 ",
		"": "ignored",
		"empty-target": ""
	}`
	channel := &Channel{
		Models:       "nanobanana,gpt-image-2,public-nano",
		ModelMapping: &modelMapping,
	}

	require.ElementsMatch(t, []string{
		"nanobanana",
		"gpt-image-2",
		"public-nano",
		"public-gpt-image",
	}, channelAbilityModels(channel))
}

func TestAddAbilitiesIncludesModelMappingSources(t *testing.T) {
	clearPreferredOwnerTables(t)

	modelMapping := `{"public-nano":"nanobanana"}`
	channel := &Channel{
		Id:           9101,
		Type:         constant.ChannelTypeMihuifang,
		Key:          "key",
		Status:       common.ChannelStatusEnabled,
		Models:       "nanobanana",
		Group:        "default",
		ModelMapping: &modelMapping,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	var models []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("model", &models).Error)
	require.ElementsMatch(t, []string{"nanobanana", "public-nano"}, models)
}

func TestGetEnabledChannelsForGroupModelIncludesMappedModelCandidates(t *testing.T) {
	clearPreferredOwnerTables(t)

	highPriority := int64(20)
	lowPriority := int64(10)
	modelMapping := `{"public-nano":"nanobanana"}`
	channels := []*Channel{
		{
			Id:           9201,
			Type:         constant.ChannelTypeMihuifang,
			Key:          "key-1",
			Status:       common.ChannelStatusEnabled,
			Models:       "nanobanana",
			Group:        "default",
			Priority:     &lowPriority,
			ModelMapping: &modelMapping,
		},
		{
			Id:       9202,
			Type:     constant.ChannelTypeGemini,
			Key:      "key-2",
			Status:   common.ChannelStatusEnabled,
			Models:   "public-nano",
			Group:    "default",
			Priority: &highPriority,
		},
	}
	for _, channel := range channels {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	got, err := GetEnabledChannelsForGroupModel("default", "public-nano")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 9202, got[0].Id)
	require.Equal(t, 9201, got[1].Id)
}

func TestEditChannelByTagRebuildsAbilitiesWhenModelMappingChanges(t *testing.T) {
	clearPreferredOwnerTables(t)

	tag := "mapped-tag"
	channel := &Channel{
		Id:     9102,
		Type:   constant.ChannelTypeMihuifang,
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Models: "nanobanana",
		Group:  "default",
		Tag:    &tag,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	modelMapping := `{"public-nano":"nanobanana"}`
	require.NoError(t, EditChannelByTag(tag, nil, &modelMapping, nil, nil, nil, nil, nil, nil))

	var models []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Pluck("model", &models).Error)
	require.ElementsMatch(t, []string{"nanobanana", "public-nano"}, models)
}
