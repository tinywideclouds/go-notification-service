// Package pipeline contains the core message processing components for the service.
package pipeline

import (
	"context"
	"fmt"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"google.golang.org/protobuf/encoding/protojson"
)

// NotificationRequestTransformer is a dataflow Transformer that safely unmarshals
// and validates a raw message payload into a structured transport.NotificationRequest.
func NotificationRequestTransformer(
	_ context.Context,
	msg *messagepipeline.Message,
) (*transport.NotificationRequest, bool, error) {
	var protoReq transport.NotificationRequestPb
	err := protojson.Unmarshal(msg.Payload, &protoReq)
	if err != nil {
		// If unmarshalling fails, we skip the message and return an error
		// so the StreamingService can Nack it.
		return nil, true, fmt.Errorf("failed to unmarshal notification request from message %s: %w", msg.ID, err)
	}

	nativeReq, err := transport.NotificationRequestFromProto(&protoReq)
	if err != nil {
		// If the protobuf can't be converted to our native struct (e.g., due to an
		// invalid URN), we also Nack the message.
		return nil, true, fmt.Errorf("failed to convert proto to native request for message %s: %w", msg.ID, err)
	}

	// On success, we pass the structured request to the next stage (the Processor)
	// and indicate that the message should not be skipped.
	return nativeReq, false, nil
}
