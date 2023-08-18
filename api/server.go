package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"net/http"
	"predictive-rds-scaler/history"
	"predictive-rds-scaler/scaler"
	"time"
)

type Server struct {
	logger  *zerolog.Logger
	history *history.History
	config  scaler.Config
}

func New(config scaler.Config, logger *zerolog.Logger, awsSession *session.Session) (*Server, error) {
	ctx := context.Background()
	dynamoDbHistory, err := history.New(ctx, logger, awsSession, config.AwsRegion, config.RdsClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB history: %v", err)
	}

	return &Server{
		logger:  logger,
		history: dynamoDbHistory,
		config:  config,
	}, nil
}

func (api *Server) Serve(port uint) error {
	r := mux.NewRouter()

	accessLoggingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			api.logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remoteAddr", r.RemoteAddr).
				Msg("Access log")

			next.ServeHTTP(w, r)
		})
	}

	// Add CORS middleware to allow requests from localhost:3000
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			next.ServeHTTP(w, r)
		})
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow any origin for WebSocket connections
		},
	}

	handleWebSocket := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}
		defer conn.Close()

		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Error reading message:", err)
				return
			}

			// Handle incoming WebSocket message
			fmt.Printf("Received message: %s\n", p)

			// Send a response back to the client
			err = conn.WriteMessage(messageType, p)
			if err != nil {
				fmt.Println("Error writing message:", err)
				return
			}
		}
	}

	r.HandleFunc("/snapshots", func(w http.ResponseWriter, r *http.Request) {
		startStr := r.URL.Query().Get("start")
		start := time.Now()
		var err error

		if startStr != "" {
			start, err = time.Parse(time.RFC3339, startStr)
			if err != nil {
				http.Error(w, fmt.Sprintf("Invalid start date format: %v", err), http.StatusBadRequest)
				return
			}
		}

		snapshots, err := api.history.GetAllSnapshots(start)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to retrieve snapshots: %v", err), http.StatusInternalServerError)
			return
		}

		prediction, err := api.history.GetPredictionSnapshots(api.config.PlanAheadTime)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to retrieve snapshots: %v", err), http.StatusInternalServerError)
			return
		}

		snapshots = append(snapshots, prediction...)

		// Marshal snapshots to JSON and write the response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(snapshots); err != nil {
			http.Error(w, fmt.Sprintf("Invalid start date format: %v", err), http.StatusBadRequest)
			return
		}
	}).Methods("GET")
	r.Use(corsMiddleware)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("ui/build"))))

	r.HandleFunc("/ws", handleWebSocket)

	http.Handle("/", accessLoggingMiddleware(r))

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), r)
	if err != nil {
		return err
	}

	return nil
}
