package history

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/rs/zerolog"
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

func (h *History) SaveItem(numReaders uint, maxCpuUtilization float64) error {
	// Calculate the TTL value (8 days from the current timestamp)
	ttl := time.Now().Add(8 * 24 * time.Hour).Unix()

	item := Item{
		Timestamp:         time.Now().Truncate(10 * time.Second).Format(time.RFC3339),
		ClusterName:       h.clusterName,
		NumReaders:        numReaders,
		MaxCpuUtilization: maxCpuUtilization,
		TTL:               ttl,
	}

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("failed to marshal DynamoDB item: %v", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	}

	_, err = h.client.PutItemWithContext(h.context, input) // Pass the context here
	if err != nil {
		return fmt.Errorf("failed to put item into DynamoDB: %v", err)
	}

	return nil
}

func (h *History) GetValue(lookupTime time.Time) (float64, uint, error) {
	// Convert to DynamoDB timestamp format (RFC3339)
	timeString := lookupTime.Truncate(10 * time.Second).Format(time.RFC3339)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("cluster_name = :name AND #ts = :ts"),
		ExpressionAttributeNames: map[string]*string{
			"#ts": aws.String("timestamp"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":name": {
				S: aws.String(h.clusterName),
			},
			":ts": {
				S: aws.String(timeString),
			},
		},
		ScanIndexForward: aws.Bool(false), // Descending order (newest first)
		Limit:            aws.Int64(1),    // Fetch only the newest item
	}

	result, err := h.client.QueryWithContext(h.context, input) // Pass the context here
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query DynamoDB: %v", err)
	}

	if len(result.Items) > 0 {
		item := Item{}
		err := dynamodbattribute.UnmarshalMap(result.Items[0], &item)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to unmarshal DynamoDB item: %v", err)
		}
		return item.MaxCpuUtilization, item.NumReaders, nil
	}

	// No value found for the last week, return 0
	return 0, 0, nil
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
