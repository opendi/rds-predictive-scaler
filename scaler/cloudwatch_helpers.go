package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"time"
)

func GetMaxCPUUtilization(rdsClient *rds.RDS, cloudWatchClient *cloudwatch.CloudWatch, clusterName string) (float64, error) {
	instances := GetReaderInstances(rdsClient, clusterName)

	maxCPUUtilization := 0.0
	for _, instance := range instances {
		if aws.StringValue(instance.DBInstanceStatus) == "available" {
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
							Stat:   aws.String("Maximum"),
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
				fmt.Printf("Instance: %s, Metric Value: %f\n", *instance.DBInstanceIdentifier, metricValue)
				if metricValue > maxCPUUtilization {
					maxCPUUtilization = metricValue
				}
			} else {
				fmt.Printf("Instance: %s, No CPU Utilization data available\n", *instance.DBInstanceIdentifier)
			}
		}
	}
	return maxCPUUtilization, nil
}
