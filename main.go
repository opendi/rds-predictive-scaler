package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"os"
	"os/signal"
	"predictive-rds-scaler/api"
	"predictive-rds-scaler/logging"
	"predictive-rds-scaler/scaler"
	"predictive-rds-scaler/types"
	"syscall"
	"time"
)

var conf = &types.Config{}

func init() {
	flag.StringVar(&conf.RdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.StringVar(&conf.InstanceNamePrefix, "instanceNamePrefix", "predictive-autoscaling-", "Prefix for reader instance names")
	flag.StringVar(&conf.AwsRegion, "awsRegion", "", "AWS region")

	flag.Float64Var(&conf.TargetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.StringVar(&conf.BoostHours, "boostHours", "", "Comma-separated list of hours to boost minInstances")
	flag.DurationVar(&conf.PlanAheadTime, "planAheadTime", 10*time.Minute, "The time to plan ahead when looking up prior CPU utilization")
	flag.UintVar(&conf.MinInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.UintVar(&conf.MaxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")

	flag.UintVar(&conf.ServerPort, "serverPort", 8041, "Port for the ui server")

	flag.Parse()
}

func main() {
	// Initialize logger
	logging.InitLogger()
	logger := logging.GetLogger()

	// Create AWS session
	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			Region: aws.String(conf.AwsRegion),
		},
	})

	if err != nil {
		logger.Error().Err(err).Msg("Failed to create AWS session")
		return
	}

	// Create broadcast channel
	broadcast := make(chan types.Broadcast)

	// Create and start the scaler
	rdsScaler, err := scaler.New(conf, logger, awsSession, broadcast)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create scaler")
		return
	}

	// Create and start the API server
	apiServer := api.New(conf, logger, broadcast)
	apiServer.OnClientConnect(initialBroadcasts(rdsScaler))

	go func() {
		err = apiServer.Serve(conf.ServerPort)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to start API server")
		}
	}()

	rdsScaler.Run()

	// Set up a channel to capture termination signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Block until a termination signal is received
	<-sigCh

	// Handle the termination signal by initiating graceful shutdown
	logger.Info().Msg("Received termination signal. Initiating graceful shutdown...")

	// Stop the API server and the scaler
	apiServer.Stop()
	rdsScaler.Stop()

	logger.Info().Msg("Shutdown complete. Exiting.")
}

func initialBroadcasts(rdsScaler *scaler.Scaler) func() []types.Broadcast {
	return func() []types.Broadcast {
		var broadcasts []types.Broadcast
		broadcasts = append(broadcasts, types.Broadcast{MessageType: "config", Data: conf})

		clusterStatusHistory := rdsScaler.GetClusterStatusHistory(24 * time.Hour)
		if clusterStatusHistory != nil {
			broadcasts = append(broadcasts, types.Broadcast{MessageType: "clusterStatusHistory", Data: clusterStatusHistory})
		}

		clusterStatusPredictionHistory := rdsScaler.GetClusterStatusPredictionHistory(24 * time.Hour)
		if clusterStatusPredictionHistory != nil {
			broadcasts = append(broadcasts, types.Broadcast{MessageType: "clusterStatusPredictionHistory", Data: clusterStatusPredictionHistory})
		}

		return broadcasts
	}
}
