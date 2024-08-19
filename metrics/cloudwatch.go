package metrics

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/rs/zerolog"
	"math"
	"predictive-rds-scaler/types"
	"time"
)

const periodInterval = 300 // 5 minutes

type Metrics struct {
	config types.Config
	logger *zerolog.Logger
	client *cloudwatch.CloudWatch
}

func New(config types.Config, logger *zerolog.Logger, awsSession *session.Session) *Metrics {
	client := cloudwatch.New(awsSession)

	return &Metrics{
		config: config,
		client: client,
		logger: logger,
	}
}

func (m *Metrics) GetCurrentInstanceUtilization(instance *rds.DBInstance) (float64, error) {
	var (
		metricValue       = 0.0
		err               error
		isStatusAvailable = *instance.DBInstanceStatus == "available"
	)

	if isStatusAvailable {
		metricValue, err = m.getMetricData(*instance.DBInstanceIdentifier, "CPUUtilization", periodInterval*time.Second)
		if err != nil {
			return 0, err
		}
	}

	return metricValue, nil
}

func (m *Metrics) GetHistoricClusterStatus(window time.Duration) (*types.ClusterStatus, error) {
	var lastWeek = time.Now().
		In(time.UTC).Add(-7 * 24 * time.Hour).Truncate(time.Second * 10)
	var rangeEnd = lastWeek.Add(window)

	statusHistory, err := m.GetClusterStatus(lastWeek, rangeEnd, window)
	if err != nil {
		return nil, err
	}

	if len(statusHistory) == 0 {
		return nil, fmt.Errorf("no historic data available: %w", err)
	}

	return statusHistory[0], nil
}

func (m *Metrics) GetClusterStatus(start time.Time, end time.Time, window time.Duration) ([]*types.ClusterStatus, error) {

	// Period must be a multiple of 60
	window = window.Truncate(time.Second * 60)
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
							Value: aws.String(m.config.RdsClusterName),
						},
					},
				},
				Period: aws.Int64(int64(window.Seconds())),
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
							Value: aws.String(m.config.RdsClusterName),
						},
					},
				},
				Period: aws.Int64(int64(window.Seconds())),
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
		StartTime:         aws.Time(start.Truncate(10 * time.Second)),
		EndTime:           aws.Time(end.Truncate(10 * time.Second)),
		ScanBy:            aws.String("TimestampAscending"),
	}

	resp, err := m.client.GetMetricData(input)
	if err != nil {
		return nil, err
	}

	var statusHistory = make([]*types.ClusterStatus, len(resp.MetricDataResults[0].Values))
	for i := 0; i < len(resp.MetricDataResults[0].Values); i++ {
		statusHistory[i] = &types.ClusterStatus{
			Identifier:            m.config.RdsClusterName,
			Timestamp:             *resp.MetricDataResults[0].Timestamps[i],
			CurrentActiveReaders:  uint(*resp.MetricDataResults[1].Values[i]),
			AverageCPUUtilization: *resp.MetricDataResults[0].Values[i],
			OptimalSize:           m.CalculateOptimalClusterSize(*resp.MetricDataResults[0].Values[i], uint(*resp.MetricDataResults[1].Values[i]), m.config.MinInstances),
		}
	}

	return statusHistory, nil
}

func (m *Metrics) getMetricData(instanceIdentifier, metricName string, window time.Duration) (float64, error) {
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
					Period: aws.Int64(int64(window.Seconds())),
					Stat:   aws.String("Average"),
				},
				ReturnData: aws.Bool(true),
			},
		},
		StartTime: aws.Time(time.Now().In(time.UTC).Add(-1 * window)), // 5 minutes ago
		EndTime:   aws.Time(time.Now().In(time.UTC)),
	}

	metricDataOutput, err := m.client.GetMetricData(metricInput)
	if err != nil {
		return 0, err
	}

	if len(metricDataOutput.MetricDataResults) > 0 {
		metricValue := aws.Float64Value(metricDataOutput.MetricDataResults[0].Values[0])
		return metricValue, nil
	}

	return 0, fmt.Errorf("no %s data available", metricName)
}

func (m *Metrics) CalculateOptimalClusterSize(utilization float64, currentReaderCount uint, minReaders uint) uint {
	targetAverageCPUUtilization := m.config.TargetCpuUtil
	numberOfServers := uint(math.Ceil(utilization * float64(currentReaderCount) / targetAverageCPUUtilization))
	return max(minReaders, min(numberOfServers, m.config.MaxInstances))
}
