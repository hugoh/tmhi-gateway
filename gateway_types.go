package tmhi

import (
	"bytes"
	"encoding/json"
	"strings"
)

// SignalData contains signal metrics.
type SignalData struct {
	Bands []string `json:"bands"`
	Bars  float64  `json:"bars"`
	CID   int      `json:"cid"`
	RSRP  int      `json:"rsrp"`
	RSRQ  int      `json:"rsrq"`
	RSSI  int      `json:"rssi"`
	SINR  int      `json:"sinr"`
}

// FourGSignal contains 4G signal information.
type FourGSignal struct {
	SignalData

	ENBID int `json:"eNBID"` //nolint:tagliatelle // matches the gateway API's actual field name
}

// FiveGSignal contains 5G signal information.
type FiveGSignal struct {
	SignalData

	AntennaUsed string `json:"antennaUsed"`
	GNBID       int    `json:"gNBID"` //nolint:tagliatelle // matches the gateway API's actual field name
}

// GenericSignalInfo contains generic signal information.
type GenericSignalInfo struct {
	APN          string `json:"apn"`
	HasIPv6      bool   `json:"hasIPv6"`
	Registration string `json:"registration"`
	Roaming      bool   `json:"roaming"`
}

// SignalResult contains complete signal information.
type SignalResult struct {
	FourG   *FourGSignal      `json:"4g"`
	FiveG   *FiveGSignal      `json:"5g"`
	Generic GenericSignalInfo `json:"generic"`
}

// StatusResult contains status check result.
//
// A status check returns a partial result rather than failing outright:
// Error records what went wrong while the other fields keep whatever
// could still be determined.
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
