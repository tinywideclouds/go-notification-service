// --- File: internal/pipeline/transformer.go ---
// Package pipeline contains the core message processing components for the service.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// NotificationRequestTransformer is a dataflow Transformer that safely unmarshals
// and validates a raw message payload into a structured notification.NotificationRequest.
//
// It uses standard encoding/json, relying on the native struct's UnmarshalJSON
// implementation to handle Protobuf deserialization and validation (e.g. URN parsing) internally.
func NotificationRequestTransformer(
	_ context.Context,
	msg *messagepipeline.Message,
) (*notification.NotificationRequest, bool, error) {
	var nativeReq notification.NotificationRequest

	// This single call performs:
	// 1. JSON Parsing
	// 2. Protobuf Unmarshalling (internal)
	// 3. Native Type Conversion (internal)
	// 4. Validation (e.g. URN parsing) (internal)
	if err := json.Unmarshal(msg.Payload, &nativeReq); err != nil {
		// If any step fails (malformed JSON, invalid URN, etc.), we return an error
		// and set skip=true so the StreamingService can handle the Nack/DLQ logic.
		return nil, true, fmt.Errorf("failed to unmarshal notification request from message %s: %w", msg.ID, err)
	}

	// On success, we pass the structured request to the next stage.
	return &nativeReq, false, nil
}
