package iec104

import (
	"time"

	"goGateway/internal/models"
)

// --- Type IDs (monitoring direction) --------------------------------------

const (
	M_SP_NA_1 byte = 1  // single-point, no time
	M_DP_NA_1 byte = 3  // double-point, no time
	M_ME_NA_1 byte = 9  // normalized value
	M_ME_NB_1 byte = 11 // scaled value (int16)
	M_ME_NC_1 byte = 13 // short float, no time
	M_IT_NA_1 byte = 15 // integrated totals
	M_SP_TB_1 byte = 30 // single-point + CP56Time2a
	M_DP_TB_1 byte = 31 // double-point + CP56Time2a
	M_ME_TF_1 byte = 36 // short float + CP56Time2a
	M_IT_TB_1 byte = 37 // integrated totals + CP56Time2a

	C_IC_NA_1 byte = 100 // general interrogation command
	C_CI_NA_1 byte = 101 // counter interrogation command
	C_CS_NA_1 byte = 103 // clock sync command
)

// --- Cause of Transmission ------------------------------------------------

const (
	COT_SPONTANEOUS byte = 3
	COT_ACTIVATION  byte = 6
	COT_ACT_CON     byte = 7
	COT_ACT_TERM    byte = 10
	COT_INTROGEN    byte = 20
)

// --- Quality bits (QDS / SIQ / DIQ) ---------------------------------------

const (
	QualityGood        = 0x00
	QualityInvalid     = 0x80 // IV — value not usable
	QualityNotTopical  = 0x40 // NT — value is old/stale
	QualitySubstituted = 0x20 // SB — value was manually substituted
	QualityBlocked     = 0x10 // BL — value update blocked
)

// seqMask enforces the 15-bit IEC-104 sequence number space (N(S)/N(R)).
const seqMask uint16 = 0x7FFF

// --- U-frame fixed encodings ----------------------------------------------

var (
	uSTARTDT_CON = []byte{0x68, 0x04, 0x0B, 0x00, 0x00, 0x00}
	uSTOPDT_CON  = []byte{0x68, 0x04, 0x23, 0x00, 0x00, 0x00}
	uTESTFR      = []byte{0x68, 0x04, 0x43, 0x00, 0x00, 0x00}
	uTESTFR_CON  = []byte{0x68, 0x04, 0x83, 0x00, 0x00, 0x00}
)

// Point represents one IEC-104 information object.
type Point struct {
	IOA       int
	TypeID    string
	Value     float64
	Quality   int
	Timestamp time.Time
}

// ServerStatus = per-instance runtime snapshot.
//
// Clients counts every TCP-accepted connection. Activated counts only the
// subset where the master has completed STARTDT — i.e., the IEC-104 protocol
// link is actually up and exchanging frames. UI uses Activated for the
// "linked" indication; Clients alone means "TCP only, protocol not started".
type ServerStatus struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Port      int    `json:"port"`
	ASDUAddr  int    `json:"asdu_addr"`
	Clients   int    `json:"clients"`
	Activated int    `json:"activated"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
}

// Status = fleet summary returned by Manager.Status.
type Status struct {
	Running   bool           `json:"running"`   // any instance up
	ListenIP  string         `json:"listen_ip"` // gateway-wide bind IP
	Points    int            `json:"points"`    // cached points (shared)
	Clients   int            `json:"clients"`   // sum across instances (TCP)
	Activated int            `json:"activated"` // sum of protocol-active links
	Servers   []ServerStatus `json:"servers"`
}

// Server is the fleet-level facade consumed by MQTT/API layers.
//
// Dispatch routes a point to exactly one IEC-104 slave (identified by its
// row id in iec104_servers). This matches the shape of signal_mappings,
// where every mapping is pinned to a single server: the worker resolves the
// target server_id from the cache and the manager forwards there only.
type Server interface {
	Start() error
	Stop() error
	Dispatch(serverID int64, p Point)
	Reload(gw models.IEC104Gateway, cfgs []models.IEC104Server) error
	Status() Status
	Snapshot() []Point
}
