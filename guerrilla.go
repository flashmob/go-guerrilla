package guerrilla

import (
	"errors"
	evbus "github.com/asaskevich/EventBus"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"sync"
)

var mainlog log.Logger

const (
	// server has just been created
	GuerrillaStateNew = iota
	// Server has been started and is running
	GuerrillaStateStarted
	// Server has just been stopped
	GuerrillaStateStopped
)

type Errors []error

// implement the Error interface
func (e Errors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}
	// multiple errors
	msg := ""
	for _, err := range e {
		msg += "\n" + err.Error()
	}
	return msg
}

type Guerrilla interface {
	Start() error
	Shutdown()
	Subscribe(topic string, fn interface{}) error
	Publish(topic string, args ...interface{})
	Unsubscribe(topic string, handler interface{}) error
}

type guerrilla struct {
	Config  AppConfig
	servers map[string]*server
	backend backends.Backend
	// guard controls access to g.servers
	guard sync.Mutex
	state int8
	bus   *evbus.EventBus
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b backends.Backend, l log.Logger) (Guerrilla, error) {
	g := &guerrilla{
		Config:  *ac, // take a local copy
		servers: make(map[string]*server, len(ac.Servers)),
		backend: b,
		bus:     evbus.New(),
	}
	mainlog = l
	if ac.LogLevel != "" {
		mainlog.SetLevel(ac.LogLevel)
	}

	g.state = GuerrillaStateNew
	err := g.makeServers()

	// subscribe for any events that may come in while running
	g.subscribeEvents()
	return g, err
}

// Instantiate servers
func (g *guerrilla) makeServers() error {
	mainlog.Debug("making servers")
	var errs Errors
	for _, sc := range g.Config.Servers {
		if _, ok := g.servers[sc.ListenInterface]; ok {
			// server already instantiated
			continue
		}
		server, err := newServer(&sc, g.backend, mainlog)
		if err != nil {
			mainlog.WithError(err).Errorf("Failed to create server [%s]", sc.ListenInterface)
			errs = append(errs, err)
		}
		if server != nil {
			g.servers[sc.ListenInterface] = server
			server.setAllowedHosts(g.Config.AllowedHosts)
		}

	}
	if len(g.servers) == 0 {
		errs = append(errs, errors.New("There are no servers that can start, please check your config"))
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// find a server by interface, retuning the index of the config and instance of server
func (g *guerrilla) findServer(iface string) (int, *server) {
	g.guard.Lock()
	defer g.guard.Unlock()
	ret := -1
	for i := range g.Config.Servers {
		if g.Config.Servers[i].ListenInterface == iface {
			server := g.servers[iface]
			ret = i
			return ret, server
		}
	}
	return ret, nil
}

func (g *guerrilla) removeServer(serverConfigIndex int, iface string) {
	g.guard.Lock()
	defer g.guard.Unlock()
	delete(g.servers, iface)
	// cut out from the slice
	g.Config.Servers = append(g.Config.Servers[:serverConfigIndex], g.Config.Servers[1:]...)
}

func (g *guerrilla) addServer(sc *ServerConfig) {
	g.guard.Lock()
	defer g.guard.Unlock()
	g.Config.Servers = append(g.Config.Servers, *sc)
	g.makeServers()
}

func (g *guerrilla) setConfig(i int, sc *ServerConfig) {
	g.guard.Lock()
	defer g.guard.Unlock()
	g.Config.Servers[i] = *sc
	g.servers[sc.ListenInterface].setConfig(sc)
}

// mapServers calls a callback on each server in g.servers map
// It locks the g.servers map before mapping
func (g *guerrilla) mapServers(callback func(*server)) map[string]*server {
	defer g.guard.Unlock()
	g.guard.Lock()
	for _, server := range g.servers {
		callback(server)
	}
	return g.servers
}

func (g *guerrilla) subscribeEvents() {

	// allowed_hosts changed, set for all servers
	g.Subscribe("config_change:allowed_hosts", func(c *AppConfig) {
		g.mapServers(func(server *server) {
			server.setAllowedHosts(c.AllowedHosts)
		})
		mainlog.Infof("allowed_hosts config changed, a new list was set")
	})

	// the main log file changed
	g.Subscribe("config_change:log_file", func(c *AppConfig) {
		mainlog.Change(c.LogFile)
		mainlog.Infof("main logging changed to [%s]", c.LogFile)
	})

	// when log level changes, apply to mainlog and server logs
	g.Subscribe("config_change:log_level", func(c *AppConfig) {
		mainlog.SetLevel(c.LogLevel)
		g.mapServers(func(server *server) {
			server.log.SetLevel(c.LogLevel)
		})
		mainlog.Infof("log level changed to [%s]", c.LogLevel)
	})

	// server config was updated
	g.Subscribe("server_change:update_config", func(sc *ServerConfig) {
		if i, _ := g.findServer(sc.ListenInterface); i != -1 {
			g.setConfig(i, sc)
		}
	})

	// add a new server to the config & start
	g.Subscribe("server_change:new_server", func(sc *ServerConfig) {
		if i, _ := g.findServer(sc.ListenInterface); i == -1 {
			// not found, lets add it
			g.addServer(sc)
			mainlog.Infof("New server added [%s]", sc.ListenInterface)
			if g.state == GuerrillaStateStarted {
				err := g.Start()
				if err != nil {
					mainlog.WithError(err).Info("Event server_change:new_server returned errors when starting")
				}
			}
		}
	})
	// start a server that already exists in the config and has been instantiated
	g.Subscribe("server_change:start_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			if server.state == ServerStateStopped || server.state == ServerStateNew {
				mainlog.Infof("Starting server [%s]", server.listenInterface)
				err := g.Start()
				if err != nil {
					mainlog.WithError(err).Info("Event server_change:start_server returned errors when starting")
				}
			}
		}
	})
	// stop running a server
	g.Subscribe("server_change:stop_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			if server.state == ServerStateRunning {
				server.Shutdown()
				mainlog.Infof("Server [%s] stopped.", sc.ListenInterface)
			}
		}
	})
	// server was removed from config
	g.Subscribe("server_change:remove_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			server.Shutdown()
			g.removeServer(i, sc.ListenInterface)
			mainlog.Infof("Server [%s] removed from config, stopped it.", sc.ListenInterface)
		}
	})

	// TLS changes
	g.Subscribe("server_change:tls_config", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			if err := server.configureSSL(); err == nil {
				mainlog.Infof("Server [%s] new TLS configuration loaded", sc.ListenInterface)
			} else {
				mainlog.WithError(err).Errorf("Server [%s] failed to load the new TLS configuration", sc.ListenInterface)
			}
		}
	})
	// when server's timeout change.
	g.Subscribe("server_change:timeout", func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			server.setTimeout(sc.Timeout)
		})
	})
	// when server's max clients change.
	g.Subscribe("server_change:max_clients", func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			// TODO resize the pool somehow
		})
	})
	// when a server's log file changes
	g.Subscribe("server_change:new_log_file", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			mainlog.Infof("Server [%s] changed, now logging to: [%s]", sc.ListenInterface, sc.LogFile)
			server.log.Change(sc.LogFile)
		}
	})
	// when the daemon caught a sighup
	g.Subscribe("server_change:reopen_log_file", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			mainlog.Infof("Server [%s] re-opened log file [%s]", sc.ListenInterface, sc.LogFile)
			server.log.Reopen()
		}
	})

}

// Entry point for the application. Starts all servers.
func (g *guerrilla) Start() error {
	var startErrors Errors
	g.guard.Lock()
	defer func() {
		g.state = GuerrillaStateStarted
		g.guard.Unlock()
	}()
	if len(g.servers) == 0 {
		return append(startErrors, errors.New("No servers to start, please check the config"))
	}
	// channel for reading errors
	errs := make(chan error, len(g.servers))
	var startWG sync.WaitGroup

	// start servers, send any errors back to errs channel
	for ListenInterface := range g.servers {
		if !g.servers[ListenInterface].isEnabled() {
			// not enabled
			continue
		}
		if g.servers[ListenInterface].state != ServerStateNew &&
			g.servers[ListenInterface].state != ServerStateStopped {
			continue
		}
		startWG.Add(1)
		go func(s *server) {
			if err := s.Start(&startWG); err != nil {
				errs <- err
			}
		}(g.servers[ListenInterface])
	}
	// wait for all servers to start (or fail)
	startWG.Wait()

	// close, then read any errors
	close(errs)
	for err := range errs {
		if err != nil {
			startErrors = append(startErrors, err)
		}
	}
	if len(startErrors) > 0 {
		return startErrors
	} else {
		if gw, ok := g.backend.(*backends.BackendGateway); ok {
			if gw.State == backends.BackendStateShuttered {
				_ = gw.Reinitialize()
			}
		}
	}
	return nil
}

func (g *guerrilla) Shutdown() {
	g.guard.Lock()
	defer func() {
		g.state = GuerrillaStateStopped
		defer g.guard.Unlock()
	}()
	for ListenInterface, s := range g.servers {
		if s.state == ServerStateRunning {
			s.Shutdown()
			mainlog.Infof("shutdown completed for [%s]", ListenInterface)
		}
	}
	if err := g.backend.Shutdown(); err != nil {
		mainlog.WithError(err).Warn("Backend failed to shutdown")
	} else {
		mainlog.Infof("Backend shutdown completed")
	}
}

func (g *guerrilla) Subscribe(topic string, fn interface{}) error {
	return g.bus.Subscribe(topic, fn)
}

func (g *guerrilla) Publish(topic string, args ...interface{}) {
	g.bus.Publish(topic, args...)
}

func (g *guerrilla) Unsubscribe(topic string, handler interface{}) error {
	return g.bus.Unsubscribe(topic, handler)
}
