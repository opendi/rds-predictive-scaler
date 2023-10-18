package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"math"
	"strconv"
	"time"
)

const periodInterval = 300 // 5 minutes interval

// GetMaxCPUUtilization returns the maximum CPU utilization among all RDS instances in the cluster.
func (s *Scaler) getMaxCPUUtilization(instanceStatus []InstanceStatus) (float64, uint, error) {
	maxCPUUtilization := 0.0
	availableReaderCount := uint(0)

	for _, status := range instanceStatus {
		if status.Status != "available" || (status.IsWriter && len(instanceStatus) > 1) {
			continue // Skip non-available instances and writers if there are other instances
		}

		maxCPUUtilization = math.Max(maxCPUUtilization, status.CPUUtilization)

		if !status.IsWriter {
			availableReaderCount++
		}
	}

	// let's not allow 0 because it can only happen when the API returned no data
	if maxCPUUtilization == 0.0 {
		maxCPUUtilization = s.config.TargetCpuUtil + 1
	}

	s.logger.Info().Float64("MaxCPUUtilization", maxCPUUtilization).Msg("Max CPU utilization")
	return maxCPUUtilization, availableReaderCount, nil
}

func (s *Scaler) getInstanceUtilization(instance *rds.DBInstance) float64 {
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

	return metricValue
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
