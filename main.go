package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws/session"
	"os"
	"os/signal"
	"predictive-rds-scaler/api"
	"predictive-rds-scaler/logging"
	"predictive-rds-scaler/scaler"
	"syscall"
	"time"
)

var config scaler.Config

func init() {
	flag.UintVar(&config.ScaleInStep, "scaleInStep", 1, "Number of reader instances to scale in at a time")
	flag.UintVar(&config.ScaleOutStep, "scaleOutStep", 1, "Number of reader instances to scale out at a time")
	flag.UintVar(&config.MinInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.UintVar(&config.MaxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")
	flag.StringVar(&config.AwsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&config.BoostHours, "boostHours", "", "Comma-separated list of hours to boost minInstances")
	flag.StringVar(&config.RdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.Float64Var(&config.TargetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.DurationVar(&config.ScaleOutCooldown, "scaleOutCooldown", 10*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")
	flag.DurationVar(&config.ScaleInCooldown, "scaleInCooldown", 5*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")
	flag.DurationVar(&config.PlanAheadTime, "planAheadTime", 10*time.Minute, "The time to plan ahead when looking up prior CPU utilization")
	flag.UintVar(&config.ServerPort, "serverPort", 8041, "Port for the ui server")

	flag.Parse()
}

func main() {

	logging.InitLogger()

	// Create the logger
	logger := logging.GetLogger()

	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create AWS session")
		return
	}

	broadcast := make(chan scaler.Broadcast)

	// Create and start the scaler
	rdsScaler, err := scaler.New(config, logger, awsSession, broadcast)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create scaler")
		return
	}

	// Create and start the API server
	apiServer, err := api.New(config, logger, awsSession, broadcast)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create API server")
		return
	}

	rdsScaler.Run()

	go func() {
		err = apiServer.Serve(config.ServerPort)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to start API server")
		}
	}()

	// Set up a channel to capture termination signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Block until a termination signal is received
	<-sigCh

	// Handle the termination signal by initiating graceful shutdown
	logger.Info().Msg("Received termination signal. Initiating graceful shutdown...")

	// Stop the API server
	apiServer.Stop()

	// Stop the scaler
	rdsScaler.Stop()

	logger.Info().Msg("Shutdown complete. Exiting.")
}
