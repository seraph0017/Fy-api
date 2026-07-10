package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelUpdateAuditSelectionDetailsRecordsPriorityAndWeight(t *testing.T) {
	oldPriority := int64(3)
	oldWeight := uint(20)
	newPriority := int64(3)
	newWeight := uint(50)

	origin := model.Channel{
		Id:       8,
		Name:     "old-channel",
		Status:   1,
		Group:    "gpt_镜工场",
		Models:   "gpt-image-2",
		Priority: &oldPriority,
		Weight:   &oldWeight,
	}
	updated := model.Channel{
		Id:       8,
		Name:     "old-channel",
		Status:   1,
		Group:    "gpt_镜工场",
		Models:   "gpt-image-2",
		Priority: &newPriority,
		Weight:   &newWeight,
	}

	changedFields, selection := buildChannelUpdateAuditSelectionDetails(updated, origin, nil)

	require.Contains(t, changedFields, "weight")
	assert.NotContains(t, changedFields, "priority")
	assert.Equal(t, map[string]interface{}{
		"status":   1,
		"group":    "gpt_镜工场",
		"models":   "gpt-image-2",
		"priority": int64(3),
		"weight":   20,
	}, selection["before"])
	assert.Equal(t, map[string]interface{}{
		"status":   1,
		"group":    "gpt_镜工场",
		"models":   "gpt-image-2",
		"priority": int64(3),
		"weight":   50,
	}, selection["after"])
}

func TestBuildChannelUpdateAuditSelectionDetailsRecordsPriorityChange(t *testing.T) {
	oldPriority := int64(1)
	weight := uint(50)
	newPriority := int64(3)

	origin := model.Channel{Priority: &oldPriority, Weight: &weight}
	updated := model.Channel{Priority: &newPriority, Weight: &weight}

	changedFields, selection := buildChannelUpdateAuditSelectionDetails(updated, origin, []string{"models"})

	require.Equal(t, []string{"models", "priority"}, changedFields)
	assert.Equal(t, int64(1), selection["before"].(map[string]interface{})["priority"])
	assert.Equal(t, int64(3), selection["after"].(map[string]interface{})["priority"])
}
