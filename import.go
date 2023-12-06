package main

import (
	"cloud.google.com/go/bigquery"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"golang.org/x/net/context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

type ClusterStatus struct {
	Timestamp      time.Time `bigquery:"timestamp"`
	ClusterID      string    `bigquery:"cluster_id"`
	CPUUtilization float64   `bigquery:"cpu_utilization"`
	InstanceCount  int64     `bigquery:"instance_count"`
}

func main() {
	// Create an AWS session.
	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}

	// Replace "YourClusterName" with the actual RDS cluster name.
	clusterName := "aurora-opendi-global-082022-cluster"
	currentTime := time.Now().In(time.UTC)
	startTime := currentTime.AddDate(0, 0, -14) // 14 days is the maximum for 1 minute intervals
	lastYear := startTime.AddDate(-1, 0, 0)     // 1 year in 1 hour intervals
	endTime := currentTime
	interval := 60 * time.Second

	// Initialize a CloudWatch client.
	cwClient := cloudwatch.New(awsSession)

	oneHourInterval := 1 * time.Hour
	historicalData, err := retrieveHistoricalData(cwClient, clusterName, lastYear, startTime, oneHourInterval)
	if err != nil {
		log.Fatalf("Failed to retrieve additional historical data: %v", err)
		return
	}

	recentData, err := retrieveHistoricalData(cwClient, clusterName, startTime, endTime, interval)
	if err != nil {
		log.Fatalf("Failed to retrieve historical data: %v", err)
		return
	}

	historicalData = append(historicalData, recentData...)

	for i := range historicalData {
		fmt.Printf("%d Time: %s, CPU Utilization: %f, Instance Count: %d\n", i, historicalData[i].Timestamp, historicalData[i].CPUUtilization, historicalData[i].InstanceCount)
	}

	ctx := context.Background()
	projectID := "opendi-global"
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create BigQuery client: %v", err)
	}
	defer func(client *bigquery.Client) {
		err := client.Close()
		if err != nil {
			log.Fatalf("Failed to close BigQuery client: %v", err)
		}
	}(client)

	datasetID := "rds_predictive_scaler"
	tableID := "history"

	datasetRef := client.Dataset(datasetID)
	tableRef := datasetRef.Table(tableID)

	//// Define a BigQuery schema that matches the ClusterStatus struct.
	//schema, err := bigquery.InferSchema(ClusterStatus{})
	//if err != nil {
	//	log.Fatalf("Failed to infer BigQuery schema: %v", err)
	//	return
	//}

	//// Create the schema in BigQuery.
	//if err := tableRef.Create(ctx, &bigquery.TableMetadata{
	//	Schema: schema,
	//}); err != nil {
	//	log.Fatalf("Failed to create BigQuery table: %v", err)
	//	return
	//}

	inserter := tableRef.Inserter()
	inserter.SkipInvalidRows = true
	inserter.IgnoreUnknownValues = true

	if err := inserter.Put(ctx, historicalData); err != nil {
		log.Fatalf("Failed to insert data into BigQuery: %v", err)
	} else {
		fmt.Println("Data successfully inserted into BigQuery.")
	}

	fmt.Printf("Total data points: %d\n", len(historicalData))
}

func retrieveHistoricalData(cwClient *cloudwatch.CloudWatch, clusterID string, startTime, endTime time.Time, interval time.Duration) ([]ClusterStatus, error) {
	var result []ClusterStatus

	// Define multiple MetricDataQueries for different metrics.
	metricQueries := []*cloudwatch.MetricDataQuery{
		{
			Id: aws.String("cpu"),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/RDS"),
					MetricName: aws.String("CPUUtilization"),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("DBClusterIdentifier"),
							Value: aws.String(clusterID),
						},
					},
				},
				Period: aws.Int64(int64(interval.Seconds())),
				Stat:   aws.String("Average"),
			},
		},
		{
			Id:         aws.String("m1"),
			ReturnData: aws.Bool(false),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/RDS"),
					MetricName: aws.String("CPUUtilization"),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("DBClusterIdentifier"),
							Value: aws.String(clusterID),
						},
					},
				},
				Period: aws.Int64(int64(interval.Seconds())),
				Stat:   aws.String("SampleCount"),
			},
		},
		{
			Id:         aws.String("instanceCount"),
			ReturnData: aws.Bool(true), // This ensures that the query result is available in the response.
			Expression: aws.String("CEIL(m1/PERIOD(m1) * 60)"),
		},
	}

	input := &cloudwatch.GetMetricDataInput{
		MetricDataQueries: metricQueries,
		StartTime:         aws.Time(startTime),
		EndTime:           aws.Time(endTime),
		ScanBy:            aws.String("TimestampAscending"),
	}

	resp, err := cwClient.GetMetricData(input)
	if err != nil {
		return nil, err
	}

	// Ensure that both MetricDataResults are present.
	if len(resp.MetricDataResults) >= 2 {
		// Iterate through the timestamps and values of the metric data results.
		for i, timestamp := range resp.MetricDataResults[0].Timestamps {
			result = append(result, ClusterStatus{
				Timestamp:      *timestamp,
				ClusterID:      clusterID,
				CPUUtilization: *resp.MetricDataResults[0].Values[i],
				InstanceCount:  int64(*resp.MetricDataResults[1].Values[i]),
			})
		}
	}

	return result, nil
}
