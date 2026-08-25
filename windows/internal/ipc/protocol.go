package ipc

const PipeName = `\\.\pipe\MasqueVpn`

const (
	CmdImport         = "import"
	CmdConnect        = "connect"
	CmdDisconnect     = "disconnect"
	CmdStatus         = "status"
	CmdSetAutoconnect = "set_autoconnect"

	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateReconnecting = "reconnecting"
	StateError        = "error"

	EventStatus = "status"
)

type Request struct {
	ID          uint64 `json:"id"`
	Cmd         string `json:"cmd"`
	Text        string `json:"text,omitempty"`
	Filename    string `json:"filename,omitempty"`
	CA          string `json:"ca,omitempty"`
	Cert        string `json:"cert,omitempty"`
	Key         string `json:"key,omitempty"`
	Autoconnect *bool  `json:"autoconnect,omitempty"`
}

type Response struct {
	ID          uint64 `json:"id"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Event       string `json:"event,omitempty"`
	State       string `json:"state"`
	Detail      string `json:"detail,omitempty"`
	Configured  bool   `json:"configured"`
	Autoconnect bool   `json:"autoconnect"`
	AssignedIP  string `json:"assigned_ip,omitempty"`
	RTTMs       int64  `json:"rtt_ms,omitempty"`
}
