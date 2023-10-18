package logging

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"predictive-rds-scaler/scaler"
)

func InitLogger(broadcast chan scaler.Broadcast) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	/*log.Logger = log.Output(zerolog.MultiLevelWriter(
		zerolog.ConsoleWriter{Out: os.Stderr},
		BroadcastChannelWriter{Channel: broadcast},
	))*/
}

func GetLogger() *zerolog.Logger {
	return &log.Logger
}

/*type BroadcastChannelWriter struct {
	Channel chan scaler.Broadcast
}

// Write implements the io.Writer interface.
func (w BroadcastChannelWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Assuming scaler.Broadcast has a field for log messages.
	broadcastMessage := scaler.Broadcast{MessageType: "logMessage", Data: msg}
	w.Channel <- broadcastMessage
	return len(p), nil
}*/
