package guerrilla

import (
	evbus "github.com/asaskevich/EventBus"
)

type Event int

const (
	// when a new config was loaded
	EventConfigNewConfig Event = iota
	// when allowed_hosts changed
	EventConfigAllowedHosts
	// when pid_file changed
	EventConfigPidFile
	// when log_file changed
	EventConfigLogFile
	// when it's time to reload the main log file
	EventConfigLogReopen
	// when log level changed
	EventConfigLogLevel
	// when the backend's config changed
	EventConfigBackendConfig
	// when a new server was added
	EventConfigServerNew
	// when an existing server was removed
	EventConfigServerRemove
	// when a new server config was detected (general event)
	EventConfigServerConfig
	// when a server was enabled
	EventConfigServerStart
	// when a server was disabled
	EventConfigServerStop
	// when a server's log file changed
	EventConfigServerLogFile
	// when it's time to reload the server's log
	EventConfigServerLogReopen
	// when a server's timeout changed
	EventConfigServerTimeout
	// when a server's max clients changed
	EventConfigServerMaxClients
	// when a server's TLS config changed
	EventConfigServerTLSConfig
)

var eventList = [...]string{
	"config_change:new_config",
	"config_change:allowed_hosts",
	"config_change:pid_file",
	"config_change:log_file",
	"config_change:reopen_log_file",
	"config_change:log_level",
	"config_change:backend_config",
	"server_change:new_server",
	"server_change:remove_server",
	"server_change:update_config",
	"server_change:start_server",
	"server_change:stop_server",
	"server_change:new_log_file",
	"server_change:reopen_log_file",
	"server_change:timeout",
	"server_change:max_clients",
	"server_change:tls_config",
}

func (e Event) String() string {
	return eventList[e]
}

type EventHandler struct {
	evbus.Bus
}

func (h *EventHandler) Subscribe(topic Event, fn interface{}) error {
	if h.Bus == nil {
		h.Bus = evbus.New()
	}
	return h.Bus.Subscribe(topic.String(), fn)
}

func (h *EventHandler) Publish(topic Event, args ...interface{}) {
	h.Bus.Publish(topic.String(), args...)
}

func (h *EventHandler) Unsubscribe(topic Event, handler interface{}) error {
	return h.Bus.Unsubscribe(topic.String(), handler)
}
