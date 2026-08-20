package daemon

type CommandRequest struct {
	Command             string `json:"command"`
	ManifestPath        string `json:"manifestPath,omitempty"`
	Name                string `json:"name,omitempty"`
	From                string `json:"from,omitempty"`
	To                  string `json:"to,omitempty"`
	ClientService       string `json:"clientService,omitempty"`
	Replica             *int   `json:"replica,omitempty"`
	AllowActiveCallback bool   `json:"allowActiveCallback,omitempty"`
	Event               string `json:"event,omitempty"`
	DeviceID            string `json:"deviceId,omitempty"`
	AllowDisruptive     bool   `json:"allowDisruptive,omitempty"`
	NoCache             bool   `json:"noCache,omitempty"`
	Force               bool   `json:"force,omitempty"`
	OutputJSON          bool   `json:"outputJson,omitempty"`
}

type CommandResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}
