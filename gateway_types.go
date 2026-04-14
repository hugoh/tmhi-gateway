package tmhi

import (
	"bytes"
	"encoding/json"
	"strings"
)

// SignalData contains signal metrics.
type SignalData struct {
	Bands []string
	Bars  float64
	CID   int
	RSRP  int
	RSRQ  int
	RSSI  int
	SINR  int
}

// FourGSignal contains 4G signal information.
type FourGSignal struct {
	SignalData

	ENBID int
}

// FiveGSignal contains 5G signal information.
type FiveGSignal struct {
	SignalData

	AntennaUsed string
	GNBID       int
}

// GenericSignalInfo contains generic signal information.
type GenericSignalInfo struct {
	APN          string
	HasIPv6      bool
	Registration string
	Roaming      bool
}

// SignalResult contains complete signal information.
type SignalResult struct {
	FourG   *FourGSignal
	FiveG   *FiveGSignal
	Generic GenericSignalInfo
}

// LoginResult contains authentication result.
type LoginResult struct {
	Success    bool
	Token      string
	Expiration int
	SessionID  string
	CSRFToken  string
}

// StatusResult contains status check result.
type StatusResult struct {
	WebInterfaceUp bool
	StatusCode     int
	Registration   string
	Error          error
}

// InfoResult contains gateway information response.
type InfoResult struct {
	Data        map[string]any
	Raw         []byte
	ContentType string
	StatusCode  int
}

func (r *InfoResult) String() string {
	if !strings.HasPrefix(r.ContentType, "application/json") {
		return string(r.Raw)
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, r.Raw, "", " "); err != nil {
		return string(r.Raw)
	}

	return prettyJSON.String()
}

// RegistrationResult contains registration status.
type RegistrationResult struct {
	Status string
}
