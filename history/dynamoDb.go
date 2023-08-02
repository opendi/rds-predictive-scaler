package history

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"time"
)

const tableName = "predictive-autoscaling-history"

type Item struct {
	Timestamp         string  `json:"timestamp"`
	NumReaders        uint    `json:"num_readers"`
	MaxCpuUtilization float64 `json:"max_cpu_utilization"`
	ClusterName       string  `json:"cluster_name"`
	TTL               int64   `json:"ttl"`
}

type DynamoDBHistory struct {
	dynamoDB    dynamodbiface.DynamoDBAPI
	clusterName string
}

func New(dynamoDB dynamodbiface.DynamoDBAPI, name string) (*DynamoDBHistory, error) {
	// Check if the table exists and create if not
	if err := createTableIfNotExists(dynamoDB); err != nil {
		return nil, err
	}

	return &DynamoDBHistory{
		dynamoDB:    dynamoDB,
		clusterName: name,
	}, nil
}

func (h *DynamoDBHistory) SaveItem(timestamp string, numReaders uint, maxCpuUtilization float64) error {
	// Calculate the TTL value (8 days from the current timestamp)
	ttl := time.Now().Add(8 * 24 * time.Hour).Unix()

	item := Item{
		Timestamp:         timestamp,
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

	_, err = h.dynamoDB.PutItem(input)
	if err != nil {
		return fmt.Errorf("failed to put item into DynamoDB: %v", err)
	}

	return nil
}

func (h *DynamoDBHistory) GetMaxCpuUtilization(lookupTime time.Time) (float64, error) {
	// Convert to DynamoDB timestamp format (RFC3339)
	timeString := lookupTime.Format(time.RFC3339)

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

	result, err := h.dynamoDB.Query(input)
	if err != nil {
		return 0, fmt.Errorf("failed to query DynamoDB: %v", err)
	}

	if len(result.Items) > 0 {
		item := Item{}
		err := dynamodbattribute.UnmarshalMap(result.Items[0], &item)
		if err != nil {
			return 0, fmt.Errorf("failed to unmarshal DynamoDB item: %v", err)
		}
		return item.MaxCpuUtilization, nil
	}

	// No value found for the last week, return 0
	return 0, nil
}

func createTableIfNotExists(dynamoDB dynamodbiface.DynamoDBAPI) error {
	tableName := aws.String(tableName)

	// Check if the table exists
	existingTables, err := dynamoDB.ListTables(&dynamodb.ListTablesInput{})
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
		fmt.Println("Creating DynamoDB table:", tableName)

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

		_, err = dynamoDB.CreateTable(input)
		if err != nil {
			return fmt.Errorf("failed to create DynamoDB table: %v", err)
		}

		// Wait for the table to be created
		fmt.Println("Waiting for the table to be created...")
		waitInput := &dynamodb.DescribeTableInput{
			TableName: tableName,
		}
		err = dynamoDB.WaitUntilTableExists(waitInput)
		if err != nil {
			return fmt.Errorf("failed to wait for table creation: %v", err)
		}

		fmt.Println("Table created successfully.")

		// Enable TTL on the table
		fmt.Println("Enabling Time to Live (TTL) for the table...")
		ttlInput := &dynamodb.UpdateTimeToLiveInput{
			TableName: tableName,
			TimeToLiveSpecification: &dynamodb.TimeToLiveSpecification{
				AttributeName: aws.String("ttl"), // The attribute that contains the TTL timestamp
				Enabled:       aws.Bool(true),    // Enable TTL
			},
		}

		_, err = dynamoDB.UpdateTimeToLive(ttlInput)
		if err != nil {
			return fmt.Errorf("failed to enable TTL for the table: %v", err)
		}

		fmt.Println("TTL enabled successfully.")
	}

	return nil
}
