package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"strconv"
	"time"
)

const periodInterval = 300 // 5 minutes interval

// GetMaxCPUUtilization returns the maximum CPU utilization among all RDS instances in the cluster.
func (s *Scaler) getMaxCPUUtilization(readerInstances []*rds.DBInstance, writerInstance *rds.DBInstance) (float64, uint, error) {
	maxCPUUtilization := 0.0
	availableReaderCount := uint(0)

	for _, instance := range readerInstances {
		var isStatusAvailable bool
		maxCPUUtilization, isStatusAvailable = s.getInstanceMetric(instance, maxCPUUtilization)

		if isStatusAvailable {
			availableReaderCount++
		}
	}

	if maxCPUUtilization == 0.0 {
		maxCPUUtilization, _ = s.getInstanceMetric(writerInstance, maxCPUUtilization)
	}

	s.logger.Info().Float64("MaxCPUUtilization", maxCPUUtilization).Msg("Max CPU utilization")
	return maxCPUUtilization, availableReaderCount, nil
}

func (s *Scaler) getInstanceMetric(instance *rds.DBInstance, maxCPUUtilization float64) (float64, bool) {
	var (
		metricValue       = 0.0
		err               error
		isStatusAvailable = *instance.DBInstanceStatus == "available"
	)

	if isStatusAvailable {
		metricValue, err = s.getMetricData(*instance.DBInstanceIdentifier, "CPUUtilization")
		if err != nil {
			s.logger.Error().Err(err).Str("InstanceID", *instance.DBInstanceIdentifier).Msg("Failed to get CPU utilization")
		}
	}

	s.logger.Info().
		Str("InstanceID", *instance.DBInstanceIdentifier).
		Str("InstanceStatus", *instance.DBInstanceStatus).
		Str("MetricValue", strconv.FormatFloat(metricValue, 'f', 2, 64)).
		Msg("Instance metrics")

	if metricValue > maxCPUUtilization {
		maxCPUUtilization = metricValue
	}
	return maxCPUUtilization, isStatusAvailable
}

// getMetricData retrieves the metric data for the given metric and DB instance.
func (s *Scaler) getMetricData(instanceIdentifier, metricName string) (float64, error) {
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

	metricDataOutput, err := s.cloudWatchClient.GetMetricData(metricInput)
	if err != nil {
		return 0, err
	}

	if len(metricDataOutput.MetricDataResults) > 0 {
		metricValue := aws.Float64Value(metricDataOutput.MetricDataResults[0].Values[0])
		return metricValue, nil
	}

	return 0, fmt.Errorf("no %s data available", metricName)
}
