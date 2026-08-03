package domain

import "time"

// SSHEvent is one line parsed from the SSH auth log (sshd via syslog).
type SSHEvent struct {
	Time    time.Time `json:"time"`
	IP      string    `json:"ip"`
	User    string    `json:"user,omitempty"`
	Action  string    `json:"action"` // accepted | failed | invalid_user | closed
	Success bool      `json:"success"`
	Detail  string    `json:"detail,omitempty"`
}

// ConnectionEvent marks the first time a remote IP was seen with an
// established TCP connection to one of the watched local ports (e.g. a game
// server port). Reconnects after the connection drops are logged again.
type ConnectionEvent struct {
	Time      time.Time `json:"time"`
	IP        string    `json:"ip"`
	LocalPort int       `json:"local_port"`
}
