package types

type Broadcast struct {
	MessageType string      `json:"type"`
	Data        interface{} `json:"data"`
}
