package observability

var (
	BootConfig    = Msg{Name: "boot.config", Required: []string{"service", "config"}}
	BootListening = Msg{Name: "boot.listening", Required: []string{"service", "addr"}}
	BootStopped   = Msg{Name: "boot.stopped", Required: []string{"service", "err"}}
)
