// --- File: internal/pipeline/transformer_test.go ---
package pipeline_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/pipeline"
	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

func TestNotificationRequestTransformer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	// Helper to create a native request
	urnObj, _ := urn.Parse("urn:contacts:user:user-123")
	validReq := &notification.NotificationRequest{
		RecipientID: urnObj,
		Content: notification.NotificationContent{
			Title: "Test",
		},
	}
	validPayload, err := json.Marshal(validReq)
	require.NoError(t, err)

	// Create a payload that looks like JSON but has an invalid URN string.
	// Since we can't easily force json.Marshal to produce an invalid URN from a typed struct,
	// we construct this JSON manually to test the validation logic inside UnmarshalJSON.
	invalidURNPayload := []byte(`{"recipientId": "not-a-valid-urn"}`)

	testCases := []struct {
		name                  string
		inputMessage          *messagepipeline.Message
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "Happy Path - Valid JSON",
			inputMessage: &messagepipeline.Message{
				MessageData: messagepipeline.MessageData{ID: "msg-1", Payload: validPayload},
			},
			expectError: false,
		},
		{
			name: "Failure - Malformed JSON",
			inputMessage: &messagepipeline.Message{
				MessageData: messagepipeline.MessageData{ID: "msg-2", Payload: []byte("not-json")},
			},
			expectError:           true,
			expectedErrorContains: "failed to unmarshal notification request",
		},
		{
			name: "Failure - Invalid URN (Validation)",
			inputMessage: &messagepipeline.Message{
				MessageData: messagepipeline.MessageData{ID: "msg-3", Payload: invalidURNPayload},
			},
			expectError: true,
			// The error message comes from urn.Parse inside the notification package
			expectedErrorContains: "failed to unmarshal notification request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, skip, err := pipeline.NotificationRequestTransformer(ctx, tc.inputMessage)

			if tc.expectError {
				require.Error(t, err)
				assert.True(t, skip)
				assert.Contains(t, err.Error(), tc.expectedErrorContains)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.False(t, skip)
				assert.NotNil(t, result)
				// Basic check to ensure it parsed correctly
				assert.Equal(t, validReq.RecipientID, result.RecipientID)
			}
		})
	}
}
