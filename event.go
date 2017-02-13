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
	// when the backend changed
	EventConfigBackendName
	// when the backend's config changed
	EventConfigBackendConfig
	// when a new server was added
	EventConfigEvServerNew
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
	"config_change:backend_name",
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
	"backend:proc_config_load",
	"backend:proc_init",
	"backend:proc_shutdown",
}

func (e Event) String() string {
	return eventList[e]
}

type EventHandler struct {
	*evbus.EventBus
}

func (h *EventHandler) Subscribe(topic Event, fn interface{}) error {
	if h.EventBus == nil {
		h.EventBus = evbus.New()
	}
	return h.EventBus.Subscribe(topic.String(), fn)
}

func (h *EventHandler) Publish(topic Event, args ...interface{}) {
	h.EventBus.Publish(topic.String(), args...)
}

func (h *EventHandler) Unsubscribe(topic Event, handler interface{}) error {
	return h.EventBus.Unsubscribe(topic.String(), handler)
}
