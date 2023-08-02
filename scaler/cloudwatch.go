package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"strings"
	"time"
)

func GetMaxCPUUtilization(rdsClient *rds.RDS, cloudWatchClient *cloudwatch.CloudWatch, clusterName string) (float64, error) {
	maxCPUUtilization := 0.0

	// Print an empty line before the table
	fmt.Println()

	// Print the header row
	fmt.Printf("%-30s%-20s%-20s\n", "Instance", "Status", "CPU Utilization (%)")
	printTableLine(30, 20, 20)

	for _, instance := range getReadyInstances(rdsClient, clusterName) {
		// Get the CPU utilization metric for the instance
		metricInput := &cloudwatch.GetMetricDataInput{
			MetricDataQueries: []*cloudwatch.MetricDataQuery{
				{
					Id: aws.String("m1"),
					MetricStat: &cloudwatch.MetricStat{
						Metric: &cloudwatch.Metric{
							Namespace:  aws.String("AWS/RDS"),
							MetricName: aws.String("CPUUtilization"),
							Dimensions: []*cloudwatch.Dimension{
								{
									Name:  aws.String("DBInstanceIdentifier"),
									Value: instance.DBInstanceIdentifier,
								},
							},
						},
						Period: aws.Int64(300), // 5 minutes interval
						Stat:   aws.String("Average"),
					},
					ReturnData: aws.Bool(true),
				},
			},
			StartTime: aws.Time(time.Now().Add(-time.Minute * 5)), // 5 minutes ago
			EndTime:   aws.Time(time.Now()),
		}

		// Retrieve the metric data from CloudWatch
		metricDataOutput, err := cloudWatchClient.GetMetricData(metricInput)
		if err != nil {
			return 0, err
		}

		if len(metricDataOutput.MetricDataResults) > 0 {
			metricValue := aws.Float64Value(metricDataOutput.MetricDataResults[0].Values[0])
			fmt.Printf("%-30s%-20s%-20.2f\n", *instance.DBInstanceIdentifier, *instance.DBInstanceStatus, metricValue)
			if metricValue > maxCPUUtilization {
				maxCPUUtilization = metricValue
			}
		} else {
			fmt.Printf("%-30s%-20s%-20s\n", *instance.DBInstanceIdentifier, *instance.DBInstanceStatus, "No CPU Utilization data available")
		}
	}

	// Print a line of "-" after the table
	printTableLine(30, 20, 20)

	// Print the last row with Max CPU Utilization
	fmt.Printf("%-30s%-20s%-20.2f\n", "Max", "", maxCPUUtilization)

	// Print an empty line after the table
	fmt.Println()

	return maxCPUUtilization, nil
}

// printTableLine prints a line of "-" with given column widths
func printTableLine(widths ...int) {
	totalWidth := 0
	for _, width := range widths {
		totalWidth += width + 1 // Add 1 for the space between columns
	}
	fmt.Println(strings.Repeat("-", totalWidth))
}
