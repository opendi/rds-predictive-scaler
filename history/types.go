package history

import (
	"context"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/rs/zerolog"
	"time"
)

type UtilizationSnapshot struct {
	Timestamp         time.Time `json:"timestamp"`
	NumReaders        uint      `json:"num_readers"`
	MaxCpuUtilization float64   `json:"max_cpu_utilization"`
	PredictedValue    bool      `json:"predicted_value"`
	FutureValue       bool      `json:"future_value"`
	ClusterName       string    `json:"cluster_name"`
	TTL               int64     `json:"ttl"`
}

type History struct {
	client      *dynamodb.DynamoDB
	clusterName string
	context     context.Context
	logger      *zerolog.Logger
}
