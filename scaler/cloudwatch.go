package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"strings"
	"time"
)

const (
	maxInstanceNameWidth = 50
	tableColumnSeparator = " | "
	periodInterval       = 300 // 5 minutes interval
)

// GetMaxCPUUtilization returns the maximum CPU utilization among all RDS instances in the cluster.
func GetMaxCPUUtilization(readerInstances []*rds.DBInstance, writerInstance *rds.DBInstance, cloudWatchClient *cloudwatch.CloudWatch) (float64, uint, error) {
	fmt.Println()
	printTableHeader()
	printTableSeparator()

	maxCPUUtilization := 0.0
	availableReaderCount := uint(0)

	for _, instance := range readerInstances {
		var isStatusAvailable bool
		maxCPUUtilization, isStatusAvailable = getInstanceMetric(instance, cloudWatchClient, maxCPUUtilization)

		if isStatusAvailable {
			availableReaderCount++
		}
	}

	if maxCPUUtilization == 0.0 {
		maxCPUUtilization, _ = getInstanceMetric(writerInstance, cloudWatchClient, maxCPUUtilization)
	}

	printTableSeparator()
	fmt.Printf("%-*s%s%-20s%s%-.2f%%\n", maxInstanceNameWidth, "Max", tableColumnSeparator, " ", tableColumnSeparator, maxCPUUtilization)
	fmt.Println()

	return maxCPUUtilization, availableReaderCount, nil
}

func getInstanceMetric(instance *rds.DBInstance, cloudWatchClient *cloudwatch.CloudWatch, maxCPUUtilization float64) (float64, bool) {
	var (
		metricValue       = 0.0
		err               error
		isStatusAvailable = *instance.DBInstanceStatus == "available"
	)

	if isStatusAvailable {
		metricValue, err = getMetricData(cloudWatchClient, *instance.DBInstanceIdentifier, "CPUUtilization")
		if err != nil {
			fmt.Printf("failed to get CPU utilization for instance %s: %v", *instance.DBInstanceIdentifier, err)
		}
	}

	fmt.Printf("%-*s%s%-20s%s%.2f%%\n", maxInstanceNameWidth, *instance.DBInstanceIdentifier, tableColumnSeparator, *instance.DBInstanceStatus, tableColumnSeparator, metricValue)

	if metricValue > maxCPUUtilization {
		maxCPUUtilization = metricValue
	}
	return maxCPUUtilization, isStatusAvailable
}

// getMetricData retrieves the metric data for the given metric and DB instance.
func getMetricData(cloudWatchClient *cloudwatch.CloudWatch, instanceIdentifier, metricName string) (float64, error) {
	metricInput := &cloudwatch.GetMetricDataInput{
		MetricDataQueries: []*cloudwatch.MetricDataQuery{
			{
				Id: aws.String("m1"),
				MetricStat: &cloudwatch.MetricStat{
					Metric: &cloudwatch.Metric{
						Namespace:  aws.String("AWS/RDS"),
						MetricName: aws.String(metricName),
						Dimensions: []*cloudwatch.Dimension{
							{
								Name:  aws.String("DBInstanceIdentifier"),
								Value: aws.String(instanceIdentifier),
							},
						},
					},
					Period: aws.Int64(periodInterval),
					Stat:   aws.String("Average"),
				},
				ReturnData: aws.Bool(true),
			},
		},
		StartTime: aws.Time(time.Now().Add(-time.Second * periodInterval)), // 5 minutes ago
		EndTime:   aws.Time(time.Now()),
	}

	metricDataOutput, err := cloudWatchClient.GetMetricData(metricInput)
	if err != nil {
		return 0, err
	}

	if len(metricDataOutput.MetricDataResults) > 0 {
		metricValue := aws.Float64Value(metricDataOutput.MetricDataResults[0].Values[0])
		return metricValue, nil
	}

	return 0, fmt.Errorf("no %s data available", metricName)
}

// printTableHeader prints the header of the CPU utilization table.
func printTableHeader() {
	fmt.Printf("%-*s%s%-20s%s%-20s\n", maxInstanceNameWidth, "Instance", tableColumnSeparator, "Status", tableColumnSeparator, "CPU Utilization")
}

// printTableSeparator prints a line of "-" with given column width.
func printTableSeparator() {
	fmt.Println(strings.Repeat("-", maxInstanceNameWidth) + tableColumnSeparator + strings.Repeat("-", 20) + tableColumnSeparator + strings.Repeat("-", 20))
}
