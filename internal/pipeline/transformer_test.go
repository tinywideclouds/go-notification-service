package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-notification-service/internal/pipeline"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestNotificationRequestTransformer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	validProto := &transport.NotificationRequestPb{
		RecipientId: "urn:sm:user:user-123",
	}
	validPayload, err := protojson.Marshal(validProto)
	require.NoError(t, err)

	protoWithInvalidURN := &transport.NotificationRequestPb{
		RecipientId: "not-a-valid-urn",
	}
	invalidURNPayload, err := protojson.Marshal(protoWithInvalidURN)
	require.NoError(t, err)

	testCases := []struct {
		name                  string
		inputMessage          *messagepipeline.Message
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "Happy Path - Valid Proto",
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
			name: "Failure - Invalid URN in Proto",
			inputMessage: &messagepipeline.Message{
				MessageData: messagepipeline.MessageData{ID: "msg-3", Payload: invalidURNPayload},
			},
			expectError:           true,
			expectedErrorContains: "failed to convert proto to native request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, skip, err := pipeline.NotificationRequestTransformer(ctx, tc.inputMessage)

			if tc.expectError {
				require.Error(t, err)
				assert.True(t, skip)
				assert.Contains(t, err.Error(), tc.expectedErrorContains)
			} else {
				assert.NoError(t, err)
				assert.False(t, skip)
			}
		})
	}
}
