package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"predictive-rds-scaler/history"
	"predictive-rds-scaler/scaler"
)

type Server struct {
	logger     *zerolog.Logger
	history    *history.History
	config     scaler.Config
	broadcast  chan scaler.Broadcast
	wsClients  map[*websocket.Conn]interface{}
	wg         *sync.WaitGroup
	shutdownCh chan struct{}
}

func New(config scaler.Config, logger *zerolog.Logger, awsSession *session.Session, channel chan scaler.Broadcast) (*Server, error) {
	ctx := context.Background()
	dynamoDbHistory, err := history.New(ctx, logger, awsSession, config.AwsRegion, config.RdsClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB history: %v", err)
	}

	return &Server{
		logger:     logger,
		history:    dynamoDbHistory,
		config:     config,
		broadcast:  channel,
		wsClients:  make(map[*websocket.Conn]interface{}),
		wg:         &sync.WaitGroup{},
		shutdownCh: make(chan struct{}),
	}, nil
}

func (api *Server) Serve(port uint) error {
	r := mux.NewRouter()
	newConnectionCh := make(chan *websocket.Conn)

	// Central goroutine to manage connections and broadcasting
	api.wg.Add(1)
	go api.manageConnections(newConnectionCh)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow any origin for WebSocket connections
		},
	}

	// serve websocket connections
	r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			api.logger.Error().Err(err).Msg("Error upgrading connection")
			return
		}
		api.logger.Info().Msgf("Client connected: %s", conn.RemoteAddr())
		newConnectionCh <- conn
	})

	// serve static files
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("ui/build"))))

	api.logger.Info().Msgf("Listening on port %d", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), r)
	if err != nil {
		return err
	}

	// Wait for OS signal to trigger graceful shutdown
	<-api.shutdownCh

	// Wait for all goroutines to finish before exiting
	api.wg.Wait()
	return nil
}

func (api *Server) manageConnections(newConnectionCh chan *websocket.Conn) {
	defer api.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-api.shutdownCh:
			api.cleanupAndReturn()
			return
		case broadcast := <-api.broadcast:
			api.logger.Debug().Msgf("Received broadcast: %+v", broadcast)
			jsonData, err := json.Marshal(broadcast)
			if err != nil {
				api.logger.Error().Err(err).Msg("Error marshaling Broadcast")
				continue
			}
			for conn := range api.wsClients {
				api.logger.Debug().Msgf("Sending broadcast to client: %s", conn.RemoteAddr())
				api.broadcastData(jsonData, conn)
			}
		case conn := <-newConnectionCh:
			api.wsClients[conn] = struct{}{}
			api.sendConfiguration(conn)
			api.sendSnapshotHistory(conn)
			api.sendPredictionHistory(conn)

			go api.handleClientMessages(conn)
		}
	}
}

func (api *Server) cleanupAndReturn() {
	for conn := range api.wsClients {
		if err := conn.Close(); err != nil {
			api.logger.Error().Err(err).Msg("Error closing connection")
		}
		delete(api.wsClients, conn)
	}
}

func (api *Server) sendSnapshotHistory(conn *websocket.Conn) {
	snapshotHistory, err := api.history.GetSnapshotTimeRange(time.Now().Add(-24*time.Hour).Truncate(10*time.Second), time.Now().Truncate(10*time.Second))
	if err != nil {
		api.logger.Error().Err(err).Msg("Failed to get snapshotHistory")
		return
	}

	jsonData, err := json.Marshal(scaler.Broadcast{MessageType: "snapshots", Data: snapshotHistory})
	if err != nil {
		api.logger.Error().Err(err).Msg("Error marshaling snapshotHistory")
		return
	}
	api.broadcastData(jsonData, conn)
}

func (api *Server) sendPredictionHistory(conn *websocket.Conn) {
	previousWeekStart := time.Now().Add(-7 * 24 * time.Hour).Add(-24 * time.Hour).Add(api.config.PlanAheadTime * time.Second).Truncate(10 * time.Second)
	previousWeekEnd := previousWeekStart.Add(24 * time.Hour)
	predictionHistory, err := api.history.GetSnapshotTimeRange(previousWeekStart, previousWeekEnd)

	if err != nil {
		api.logger.Error().Err(err).Msg("Failed to get predictionHistory")
		return
	}

	for i := range predictionHistory {
		predictionHistory[i].Timestamp = predictionHistory[i].Timestamp.Add(7 * 24 * time.Hour).Add(-api.config.PlanAheadTime * time.Second).Truncate(10 * time.Second)
	}

	jsonData, err := json.Marshal(scaler.Broadcast{MessageType: "predictions", Data: predictionHistory})
	if err != nil {
		api.logger.Error().Err(err).Msg("Error marshaling predictionHistory")
		return
	}
	api.broadcastData(jsonData, conn)
}

func (api *Server) broadcastData(jsonData []byte, conn *websocket.Conn) {
	if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
		api.logger.Error().Err(err).Msg("Failed to write message")
		if err := conn.Close(); err != nil {
			api.logger.Error().Err(err).Msg("Failed to close connection")
		}
		delete(api.wsClients, conn)
	}
}

func (api *Server) sendConfiguration(conn *websocket.Conn) {
	jsonData, err := json.Marshal(scaler.Broadcast{MessageType: "config", Data: api.config})
	if err != nil {
		api.logger.Error().Err(err).Msg("Error marshaling snapshotHistory")
		return
	}
	api.broadcastData(jsonData, conn)
}

func (api *Server) handleClientMessages(conn *websocket.Conn) {
	defer func() {
		// Remove the connection from the map when this goroutine exits
		delete(api.wsClients, conn)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				api.logger.Error().Err(err).Msg("Error reading WebSocket message")
			}
			break
		}

		// Handle the received message here
		api.handleMessage(message)
	}
}

func (api *Server) handleMessage(message []byte) {
	var receivedData scaler.Broadcast
	err := json.Unmarshal(message, &receivedData)
	if err != nil {
		api.logger.Error().Err(err).Msg("Error unmarshalling WebSocket message")
		return
	}

	switch receivedData.MessageType {
	case "config_update":
		config, ok := receivedData.Data.(scaler.Config)
		if !ok {
			api.logger.Error().Msg("Failed to cast data to Config")
			return
		}

		api.config = config
		api.logger.Info().Msgf("Received configuration update: %+v", api.config)

	default:
		api.logger.Warn().Msg("Received an unsupported message type")
	}
}

func (api *Server) Stop() {
	api.logger.Info().Msg("Stopping API server")
	close(api.shutdownCh)
}
