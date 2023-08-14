package scaler

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/rs/zerolog"
	"predictive-rds-scaler/history"
	"strconv"
	"time"
)

const readerNamePrefix = "predictive-autoscaling-"

func New(config Config, logger *zerolog.Logger, awsSession *session.Session) (*Scaler, error) {
	rdsClient := rds.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	cloudWatchClient := cloudwatch.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	ctx := context.Background()
	dynamoDbHistory, err := history.New(ctx, logger, awsSession, config.AwsRegion, config.RdsClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB history: %v", err)
	}

	return &Scaler{
		config:           config,
		scaleOutStatus:   Cooldown{},
		scaleInStatus:    Cooldown{},
		rdsClient:        rdsClient,
		cloudWatchClient: cloudWatchClient,
		dynamoDbHistory:  dynamoDbHistory,
		logger:           logger,
	}, nil
}

func (s *Scaler) Run() {
	ticker := time.NewTicker(10 * time.Second)
	boostHours, err := parseBoostHours(s.config.BoostHours)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error parsing scale out hours")
		return
	}

	// Retrieve scale-out and scale-in LastTime values from cluster tags
	s.scaleOutStatus, err = s.getLastTimeFromClusterTags("ScaleOutLastTime", s.config.ScaleOutCooldown)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Error retrieving scale-out LastTime value from cluster tags")
	}

	s.scaleInStatus, err = s.getLastTimeFromClusterTags("ScaleInLastTime", s.config.ScaleInCooldown)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Error retrieving scale-in LastTime value from cluster tags")
	}

	for range ticker.C {
		writerInstance, err := s.getWriterInstance()
		if err != nil {
			s.logger.Error().Err(err).Msg("Error getting writer instance")
			continue
		}

		readerInstances, currentSize, err := s.getReaderInstances(StatusAll ^ StatusDeleting)
		if err != nil {
			s.logger.Error().Err(err).Msg("Error getting reader instances")
			continue
		}

		cpuUtilization, err := s.getUtilization(readerInstances, writerInstance)
		if err != nil {
			s.logger.Error().Err(err).Msg("Error getting CPU utilization")
			continue
		}

		minInstances := s.config.MinInstances
		if isBoostHour(time.Now().Hour(), boostHours) {
			minInstances = s.config.MinInstances + s.config.ScaleOutStep
		}

		s.logger.Info().
			Str("CPUUtilization", strconv.FormatFloat(cpuUtilization, 'f', 2, 64)).
			Uint("CurrentReaders", currentSize).
			Int("ScaleOutCooldownRemaining", int(calculateRemainingCooldown(s.config.ScaleOutCooldown, s.scaleOutStatus.LastTime).Seconds())).
			Int("ScaleInCooldownRemaining", int(calculateRemainingCooldown(s.config.ScaleInCooldown, s.scaleInStatus.LastTime).Seconds())).
			Msg("Scaler status")

		if !s.scaleOutStatus.InCooldown && s.shouldScaleOut(cpuUtilization, currentSize, minInstances) {
			scaleOutInstances := s.calculateScaleOutReaderCount(currentSize)
			if scaleOutInstances > 0 {
				s.logger.Info().Uint("ScaleOutInstances", scaleOutInstances).Msg("Scaling out instances")
				err := s.scaleOut(readerNamePrefix, scaleOutInstances)
				if err != nil {
					s.logger.Error().Err(err).Msg("Error scaling out")
				} else {
					s.scaleOutStatus.InCooldown = true
					s.scaleOutStatus.LastTime = time.Now()

					err := s.storeLastTimeInClusterTags("ScaleOutLastTime", s.scaleOutStatus.LastTime)
					if err != nil {
						s.logger.Warn().Err(err).Msg("Error storing scale-out LastTime value in cluster tags")
					}
					time.AfterFunc(s.config.ScaleOutCooldown, func() {
						s.scaleOutStatus.InCooldown = false
					})
				}
			} else {
				s.logger.Info().Msg("Max instances reached. Cannot scale out.")
			}
		} else if !s.scaleInStatus.InCooldown && !s.scaleOutStatus.InCooldown && s.shouldScaleIn(cpuUtilization, currentSize, minInstances) {
			scaleInInstances := s.calculateScaleInReaderCount(currentSize, minInstances)
			if scaleInInstances > 0 {
				s.logger.Info().Uint("ScaleInInstances", scaleInInstances).Msg("Scaling in instances")
				err := s.scaleIn(scaleInInstances)
				if err != nil {
					s.logger.Error().Err(err).Msg("Error scaling in")
				} else {
					s.scaleInStatus.InCooldown = true
					s.scaleInStatus.LastTime = time.Now()
					err := s.storeLastTimeInClusterTags("ScaleInLastTime", s.scaleInStatus.LastTime)
					if err != nil {
						s.logger.Warn().Err(err).Msg("Error storing scale-out LastTime value in cluster tags")
					}
					time.AfterFunc(s.config.ScaleInCooldown, func() {
						s.scaleInStatus.InCooldown = false
					})
				}
			} else {
				s.logger.Info().Msg("Min instances reached. Cannot scale in.")
			}
		}
	}
}

func (s *Scaler) getUtilization(readerInstances []*rds.DBInstance, writerInstance *rds.DBInstance) (float64, error) {
	historicQueryTime := time.Now().Add(-time.Hour * 24 * 7).Add(s.config.PlanAheadTime) // last week, 10 minutes into the future
	historicCpuUtilization, historicReaderCount, err := s.dynamoDbHistory.GetValue(historicQueryTime)
	if err != nil {
		return 0, fmt.Errorf("error getting historic value: %v", err)
	}

	currentCpuUtilization, currentActiveReaderCount, err := s.getMaxCPUUtilization(readerInstances, writerInstance)
	if err != nil {
		return 0, fmt.Errorf("error getting max CPU utilization: %v", err)
	}

	// Save the item to DynamoDB when scaling is required
	if currentActiveReaderCount > 0 || currentCpuUtilization > s.config.TargetCpuUtil {
		if err := s.dynamoDbHistory.SaveItem(currentActiveReaderCount, currentCpuUtilization); err != nil {
			return 0, fmt.Errorf("error saving item to DynamoDB: %v", err)
		}
	}

	if historicCpuUtilization != 0 {
		// how high would the historic CPU utilization be with the current reader count?
		predictedCpuUtilization := historicCpuUtilization * (float64(historicReaderCount+1) / float64(currentActiveReaderCount+1))
		s.logger.Info().
			Float64("HistoricCpuUtilization", historicCpuUtilization).
			Uint("HistoricReaderCount", historicReaderCount).
			Float64("PredictedCpuUtilization", predictedCpuUtilization).
			Float64("CurrentCPUUtilization", currentCpuUtilization).
			Msg("Historic value found.")
		if predictedCpuUtilization > currentCpuUtilization {
			currentCpuUtilization = predictedCpuUtilization
		}
	}
	return currentCpuUtilization, nil
}

func (s *Scaler) scaleOut(readerNamePrefix string, numInstances uint) error {
	currentHour := time.Now().Hour()
	newReaderInstanceNames := make([]string, numInstances)

	startingInstances, numStartingInstances, err := s.getReaderInstances(StatusCreating | StatusConfiguringEnhancedMonitoring)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Error getting starting instances")
	}

	if numStartingInstances > 0 {
		s.logger.Info().Msg("Waiting for starting instances to be ready")
		instanceIdentifiers := make([]string, len(startingInstances))
		for i, instance := range startingInstances {
			instanceIdentifiers[i] = *instance.DBInstanceIdentifier
		}

		err = s.waitForInstancesAvailable(instanceIdentifiers)
		if err != nil {
			s.logger.Warn().Err(err).Msg("error waiting for starting instances to be ready")
		}
		numInstances -= numStartingInstances
	}

	for i := 0; i < int(numInstances); i++ {
		// Get the current writer instance
		writerInstance, err := s.getWriterInstance()
		if err != nil {
			return fmt.Errorf("failed to get current writer instance: %v", err)
		}

		// Generate a random UID for the new reader instance name
		randomUID := generateRandomUID()

		// Create the reader instance name with the prefix, current scale-out hour, and random UID
		readerName := fmt.Sprintf("%s%d-%s", readerNamePrefix, currentHour, randomUID)

		// Use the writer instance's configuration as a template for the new reader instance
		readerDBInstance := &rds.CreateDBInstanceInput{
			DBInstanceClass:         writerInstance.DBInstanceClass,
			Engine:                  writerInstance.Engine,
			DBClusterIdentifier:     aws.String(s.config.RdsClusterName),
			DBInstanceIdentifier:    aws.String(readerName),
			PubliclyAccessible:      aws.Bool(false),
			MultiAZ:                 writerInstance.MultiAZ,
			CopyTagsToSnapshot:      writerInstance.CopyTagsToSnapshot,
			AutoMinorVersionUpgrade: writerInstance.AutoMinorVersionUpgrade,
			DBParameterGroupName:    writerInstance.DBParameterGroups[0].DBParameterGroupName,
		}

		// Perform the scaling operation to add a reader to the cluster
		_, err = s.rdsClient.CreateDBInstance(readerDBInstance)
		if err != nil {
			return fmt.Errorf("failed to add reader instance: %v", err)
		}

		s.logger.Info().Str("NewReaderInstanceName", readerName).Msg("Scaling out operation successful")

		// Add the new reader instance name to the slice
		newReaderInstanceNames[i] = readerName
	}

	// Wait for all new reader instances to become "Available"
	s.logger.Info().Msg("Waiting for all new reader instances to become 'Available'...")
	err = s.waitForInstancesAvailable(newReaderInstanceNames)
	if err != nil {
		return fmt.Errorf("failed to wait for the new reader instances to become 'Available': %v", err)
	}

	s.logger.Info().Msg("All new reader instances are now 'Available'. Continuing...")
	return nil
}

func (s *Scaler) scaleIn(numInstances uint) error {
	readerInstances, _, err := s.getReaderInstances(StatusAll)
	if err != nil {
		return fmt.Errorf("failed to get reader instances: %v", err)
	}

	for i := 0; i < int(numInstances); i++ {
		// Check if there are any reader instances available to scale in
		if len(readerInstances) == 0 {
			break
		}

		// Choose a reader instance to remove
		instance := readerInstances[0]

		// Check if the instance is in the process of deletion, and it's the last remaining reader instance
		if *instance.DBInstanceStatus == "deleting" && len(readerInstances) == 1 {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("The last remaining instance is already in status 'deleting'. Will not remove it to avoid service disruption.")
			break
		}

		// Skip over instances with the status "deleting"
		if *instance.DBInstanceStatus == "deleting" {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("Skipping instance already in status 'deleting'")
			numInstances++
			readerInstances = readerInstances[1:]
			continue
		}

		// Wait for the instance to become deletable
		s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Str("InstanceStatus", *instance.DBInstanceStatus).Msg("Waiting for the instance to become deletable")
		err := s.waitUntilInstanceDeletable(*instance.DBInstanceIdentifier)
		if err != nil {
			return fmt.Errorf("failed to wait for instance to become deletable: %v", err)
		}

		// Remove the reader instance
		_, err = s.rdsClient.DeleteDBInstance(&rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: instance.DBInstanceIdentifier,
			SkipFinalSnapshot:    aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to remove reader instance: %v", err)
		}

		s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("Scaling in operation successful")

		// Remove the scaled-in instance from the list
		readerInstances = readerInstances[1:]
	}

	return nil
}

// shouldScaleOut returns true if scaling out is needed based on the current CPU utilization and the maximum number of instances.
func (s *Scaler) shouldScaleOut(cpuUtilization float64, currentSize, minInstances uint) bool {
	if currentSize < minInstances {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", minInstances).
			Msg("Should scale out, currently below minimum instances")
		return true
	}

	if cpuUtilization > s.config.TargetCpuUtil && currentSize < s.config.MaxInstances {
		s.logger.Info().
			Float64("CPUUtilization", cpuUtilization).
			Uint("CurrentSize", currentSize).
			Uint("MaxInstances", s.config.MaxInstances).
			Msg("Should scale out, currently above target CPU utilization")
		return true
	}

	s.logger.Info().Msg("No need to scale out")
	return false
}

// calculateScaleOutReaderCount calculates the number of instances to scale out based on the maximum number of instances and the current size.
func (s *Scaler) calculateScaleOutReaderCount(currentSize uint) uint {
	return minInt(s.config.ScaleOutStep, s.config.MaxInstances-currentSize)
}

// shouldScaleIn returns true if scaling in is needed based on the current CPU utilization and the minimum number of instances.
func (s *Scaler) shouldScaleIn(currentCpuUtilization float64, currentSize, minInstances uint) bool {
	if currentCpuUtilization > s.config.TargetCpuUtil {
		return false
	}

	if currentSize < minInstances+s.config.ScaleInStep {
		s.logger.Info().Msg("Skipping scale in: Minimum instance threshold reached.")
		return false
	}

	if (currentSize - s.config.ScaleInStep) == 0 {
		if currentCpuUtilization <= 50 {
			s.logger.Info().Msg("Should scale in, CPU utilization is below 45%, scaling in to 0 reader instances.")
			return true
		} else {
			return false
		}
	}

	predictedCpuUtilization := (currentCpuUtilization * float64(currentSize)) / float64(currentSize-s.config.ScaleInStep)
	if predictedCpuUtilization == 0 {
		s.logger.Debug().
			Int("currentSize", int(currentSize)).
			Int("scaleInStep", int(s.config.ScaleInStep)).
			Float64("currentCpuUtilization", currentCpuUtilization).
			Float64("predictedCPUUtilization", predictedCpuUtilization).
			Msg("Skipping scale in: Predicted CPU utilization is 0, somebody messed up the math.")
		return false
	}

	if predictedCpuUtilization <= s.config.TargetCpuUtil {
		s.logger.Info().
			Float64("PredictedCPUUtilization", predictedCpuUtilization).
			Msg("Should scale in, predicted CPU utilization is below target.")
		return true
	}

	s.logger.Info().Msg("No need to scale in.")
	return false
}

// calculateScaleInReaderCount calculates the number of instances to scale in based on the current size and the minimum number of instances.
func (s *Scaler) calculateScaleInReaderCount(currentSize, minInstances uint) uint {
	return minInt(s.config.ScaleInStep, currentSize-minInstances)
}

func (s *Scaler) getLastTimeFromClusterTags(tagName string, cooldownDuration time.Duration) (Cooldown, error) {
	cooldown := Cooldown{}

	clusterArn, err := s.getClusterArn()
	if err != nil {
		return cooldown, err
	}

	tags, err := s.getClusterTags(clusterArn)
	if err != nil {
		return cooldown, err
	}

	lastTimeStr, ok := tags[tagName]
	if !ok {
		return cooldown, nil // Tag not found, return empty cooldown
	}

	unixTimestamp, err := strconv.ParseInt(lastTimeStr, 10, 64)
	if err != nil {
		return cooldown, fmt.Errorf("failed to parse LastTime from tag: %v", err)
	}

	lastTime := time.Unix(unixTimestamp, 0)

	cooldown.LastTime = lastTime
	cooldown.InCooldown = time.Since(lastTime) <= cooldownDuration
	return cooldown, nil
}
