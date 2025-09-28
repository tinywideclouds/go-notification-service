package cmd

import "github.com/illmade-knight/go-dataflow/pkg/messagepipeline"

// YamlConfig defines the structure for unmarshaling the embedded config.yaml file.
type YamlConfig struct {
	ProjectID              string `yaml:"project_id"`
	ListenAddr             string `yaml:"listen_addr"`
	SubscriptionID         string `yaml:"subscription_id"`
	SubscriptionDLQTopicID string `yaml:"subscription_dlq_topic_id"`
	NumPipelineWorkers     int    `yaml:"num_pipeline_workers"`
}

// AppConfig holds the final, validated configuration for the application.
type AppConfig struct {
	ProjectID              string
	ListenAddr             string
	SubscriptionID         string
	SubscriptionDLQTopicID string
	NumPipelineWorkers     int
	PubsubConsumerConfig   *messagepipeline.GooglePubsubConsumerConfig
}
