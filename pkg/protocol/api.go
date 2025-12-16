package protocol

// API keys supported by Kafscale in milestone 1.
const (
	APIKeyProduce    int16 = 0
	APIKeyFetch      int16 = 1
	APIKeyMetadata   int16 = 3
	APIKeyApiVersion int16 = 18
)

// ApiVersion describes the supported version range for an API.
type ApiVersion struct {
	APIKey     int16
	MinVersion int16
	MaxVersion int16
}
