package history

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/rs/zerolog"
	"sort"
	"time"
)

const tableName = "predictive-autoscaling-history"

func New(ctx context.Context, logger *zerolog.Logger, awsSession *session.Session, awsRegion string, name string) (*History, error) {
	dynamoDbClient := dynamodb.New(awsSession, &aws.Config{
		Region: aws.String(awsRegion),
	})

	// Check if the table exists and create if not
	if err := createTableIfNotExists(ctx, dynamoDbClient, logger); err != nil {
		return nil, err
	}

	return &History{
		client:      dynamoDbClient,
		clusterName: name,
		context:     ctx,
		logger:      logger,
	}, nil
}

func (h *History) SaveItem(snapshot *UtilizationSnapshot) error {

	av, err := dynamodbattribute.MarshalMap(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal DynamoDB snapshot: %v", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName), // Make sure `tableName` is defined and correct
		Item:      av,
	}

	// Use context.TODO() if you're not using a specific context for this operation
	_, err = h.client.PutItemWithContext(h.context, input)
	if err != nil {
		return fmt.Errorf("failed to put snapshot into DynamoDB: %v", err)
	}

	return nil // Return a pointer to the snapshot
}

func (h *History) GetValue(lookupTime time.Time, window time.Duration) (*UtilizationSnapshot, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("cluster_name-timestamp-index"),
		KeyConditionExpression: aws.String("cluster_name = :name AND #timestamp BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":name": {
				S: aws.String(h.clusterName),
			},
			":start": {
				S: aws.String(lookupTime.Format(time.RFC3339)),
			},
			":end": {
				S: aws.String(lookupTime.Add(window).Format(time.RFC3339)),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#timestamp": aws.String("timestamp"),
		},
	}

	result, err := h.client.QueryWithContext(h.context, input) // Pass the context here
	if err != nil {
		return nil, fmt.Errorf("failed to query DynamoDB: %v", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var snapshots []*UtilizationSnapshot

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var cpuUtilizations []float64

	for _, item := range result.Items {
		snapshot := UtilizationSnapshot{}
		err := dynamodbattribute.UnmarshalMap(item, &snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal DynamoDB snapshot: %v", err)
		}

		cpuUtilizations = append(cpuUtilizations, snapshot.MaxCpuUtilization)
		snapshots = append(snapshots, &snapshot)
	}

	if len(cpuUtilizations) == 0 {
		return nil, fmt.Errorf("no valid snapshots found")
	}

	sort.Float64s(cpuUtilizations)

	index := int(0.98 * float64(len(cpuUtilizations)))

	// Find the 98th percentile value
	var percentileValue float64
	if index < len(cpuUtilizations)-1 {
		percentileValue = cpuUtilizations[index] + float64(index%100)/100*(cpuUtilizations[index+1]-cpuUtilizations[index])
	} else {
		percentileValue = cpuUtilizations[len(cpuUtilizations)-1]
	}

	// Find the snapshot corresponding to the percentile value
	var snapshotWithPercentile *UtilizationSnapshot
	for _, snap := range snapshots {
		if snap.MaxCpuUtilization >= percentileValue {
			snapshotWithPercentile = snap
			break
		}
	}

	if snapshotWithPercentile == nil {
		return nil, fmt.Errorf("no snapshot found for the percentile value")
	}

	return snapshotWithPercentile, nil
}

func (h *History) GetAllSnapshots(start time.Time) ([]UtilizationSnapshot, error) {
	// Calculate the timestamp for 24 hours ago
	oneDayAgo := start.Add(-24 * time.Hour).Truncate(10 * time.Second) // Truncate to remove fractional seconds

	input := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("cluster_name-timestamp-index"),
		KeyConditionExpression: aws.String("cluster_name = :name AND #timestamp >= :time"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":name": {
				S: aws.String(h.clusterName),
			},
			":time": {
				S: aws.String(oneDayAgo.Format(time.RFC3339)),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#timestamp": aws.String("timestamp"),
		},
	}

	result, err := h.client.QueryWithContext(h.context, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query DynamoDB: %v", err)
	}

	snapshots := make([]UtilizationSnapshot, len(result.Items))
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &snapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal DynamoDB snapshots: %v", err)
	}

	for i := range snapshots {
		snapshots[i].Timestamp = snapshots[i].Timestamp.Truncate(10 * time.Second)
	}
	// Sort snapshots by timestamp in ascending order
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.Before(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

func (h *History) GetSnapshotTimeRange(start time.Time, end time.Time) ([]UtilizationSnapshot, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("cluster_name-timestamp-index"),
		KeyConditionExpression: aws.String("cluster_name = :name AND #timestamp BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":name": {
				S: aws.String(h.clusterName),
			},
			":start": {
				S: aws.String(start.Format(time.RFC3339)),
			},
			":end": {
				S: aws.String(end.Format(time.RFC3339)),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#timestamp": aws.String("timestamp"),
		},
	}

	result, err := h.client.QueryWithContext(h.context, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query DynamoDB: %v", err)
	}

	snapshots := make([]UtilizationSnapshot, len(result.Items))
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &snapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal DynamoDB snapshots: %v", err)
	}

	// Sort snapshots by timestamp in ascending order
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.Before(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

func createTableIfNotExists(ctx context.Context, client *dynamodb.DynamoDB, logger *zerolog.Logger) error {
	tableName := aws.String(tableName)

	// Check if the table exists
	existingTables, err := client.ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		return fmt.Errorf("failed to list DynamoDB tables: %v", err)
	}

	var tableExists bool
	for _, t := range existingTables.TableNames {
		if *t == *tableName {
			tableExists = true
			break
		}
	}

	// Create the table if it doesn't exist
	if !tableExists {
		logger.Info().Str("TableName", *tableName).Msg("Creating DynamoDB table")

		// Define the table schema and set billing mode to PAY_PER_REQUEST
		input := &dynamodb.CreateTableInput{
			AttributeDefinitions: []*dynamodb.AttributeDefinition{
				{
					AttributeName: aws.String("timestamp"),
					AttributeType: aws.String("S"),
				},
				{
					AttributeName: aws.String("cluster_name"),
					AttributeType: aws.String("S"),
				},
			},
			KeySchema: []*dynamodb.KeySchemaElement{
				{
					AttributeName: aws.String("timestamp"),
					KeyType:       aws.String("HASH"), // Partition key
				},
				{
					AttributeName: aws.String("cluster_name"),
					KeyType:       aws.String("RANGE"), // Sort key
				},
			},
			ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(1),
				WriteCapacityUnits: aws.Int64(1),
			},
			//BillingMode: aws.String("PAY_PER_REQUEST"), // Set billing mode to PAY_PER_REQUEST (no provisioning)
			TableName: tableName,
		}

		_, err = client.CreateTableWithContext(ctx, input) // Pass the context here
		if err != nil {
			return fmt.Errorf("failed to create DynamoDB table: %v", err)
		}

		logger.Info().Str("TableName", *tableName).Msg("Waiting for the table to be created...")

		waitInput := &dynamodb.DescribeTableInput{
			TableName: tableName,
		}
		err = client.WaitUntilTableExistsWithContext(ctx, waitInput) // Pass the context here
		if err != nil {
			return fmt.Errorf("failed to wait for table creation: %v", err)
		}

		logger.Info().Str("TableName", *tableName).Msg("Table created successfully.")
		logger.Info().Str("TableName", *tableName).Msg("Enabling Time to Live (TTL) for the table...")

		ttlInput := &dynamodb.UpdateTimeToLiveInput{
			TableName: tableName,
			TimeToLiveSpecification: &dynamodb.TimeToLiveSpecification{
				AttributeName: aws.String("ttl"), // The attribute that contains the TTL timestamp
				Enabled:       aws.Bool(true),    // Enable TTL
			},
		}

		_, err = client.UpdateTimeToLiveWithContext(ctx, ttlInput) // Pass the context here
		if err != nil {
			return fmt.Errorf("failed to enable TTL for the table: %v", err)
		}

		logger.Info().Str("TableName", *tableName).Msg("TTL enabled successfully.")
	}

	return nil
}
