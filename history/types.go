package history

import (
	"context"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/rs/zerolog"
)

type Item struct {
	Timestamp         string  `json:"timestamp"`
	NumReaders        uint    `json:"num_readers"`
	MaxCpuUtilization float64 `json:"max_cpu_utilization"`
	ClusterName       string  `json:"cluster_name"`
	TTL               int64   `json:"ttl"`
}

type History struct {
	client      *dynamodb.DynamoDB
	clusterName string
	context     context.Context
	logger      *zerolog.Logger
}
