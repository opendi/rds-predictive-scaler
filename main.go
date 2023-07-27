package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"log"
	"os"
	"predictive-rds-scaler/scaler"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

var (
	scaleOutHoursStr string
	awsRegion        string
	rdsClusterName   string
)

const readerNamePrefix = "predictive-autoscaling-"

func init() {
	flag.StringVar(&scaleOutHoursStr, "scaleOutHours", "", "Comma-separated hours for scaling out (e.g., '7,9,10')")
	flag.StringVar(&awsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&rdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.Parse()
}

func main() {
	if scaleOutHoursStr == "" {
		scaleOutHoursStr = os.Getenv("SCALE_OUT_HOURS")
	}

	if awsRegion == "" {
		awsRegion = os.Getenv("AWS_REGION")
	}

	if rdsClusterName == "" {
		rdsClusterName = os.Getenv("RDS_CLUSTER_NAME")
	}

	// Parse scale out hours from the environment variable
	scaleOutHours, err := scaler.ParseScaleOutHours(scaleOutHoursStr)
	if err != nil {
		log.Fatalf("Error parsing scale out hours: %v", err)
	}

	// Print configuration for verification
	fmt.Printf("Scale Out Hours: %v\nAWS Region: %s\nRDS Cluster Name: %s\n",
		scaleOutHours, awsRegion, rdsClusterName)

	// Create an AWS session
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}

	// Create an RDS service client
	rdsClient := rds.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	// Start the main loop for scaling
	for {
		currentHour := time.Now().Hour()

		// Check if the current hour is in the scale out hours list
		if scaler.IsScaleOutHour(currentHour, scaleOutHours) {
			// Perform scaling out operation here
			fmt.Printf("Scaling out at hour %d\n", currentHour)
			if err := scaler.ScaleOut(rdsClient, rdsClusterName, readerNamePrefix); err != nil {
				log.Printf("Error scaling out: %v\n", err)
			}
		} else if scaler.IsScaleInHour(currentHour, scaleOutHours) {
			// Perform scaling in operation here
			fmt.Printf("Scaling in at hour %d\n", currentHour)
			if err := scaler.ScaleIn(rdsClient, rdsClusterName, readerNamePrefix); err != nil {
				log.Printf("Error scaling in: %v\n", err)
			}
		}
		scaler.SleepUntilNextHour()
	}
}
