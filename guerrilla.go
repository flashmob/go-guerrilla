package guerrilla

import (
	"errors"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"sync"
	"sync/atomic"
)

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
	Subscribe(topic Event, fn interface{}) error
	Publish(topic Event, args ...interface{})
	Unsubscribe(topic Event, handler interface{}) error
	SetLogger(log.Logger)
}

type guerrilla struct {
	Config  AppConfig
	servers map[string]*server
	backend backends.Backend
	// guard controls access to g.servers
	guard sync.Mutex
	state int8
	EventHandler
	logStore
}

type logStore struct {
	atomic.Value
}

// Get loads the log.logger in an atomic operation. Returns a stderr logger if not able to load
func (ls *logStore) mainlog() log.Logger {
	if v, ok := ls.Load().(log.Logger); ok {
		return v
	}
	l, _ := log.GetLogger(log.OutputStderr.String())
	return l
}

// storeMainlog stores the log value in an atomic operation
func (ls *logStore) storeMainlog(log log.Logger) {
	ls.Store(log)
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b backends.Backend, l log.Logger) (Guerrilla, error) {
	g := &guerrilla{
		Config:  *ac, // take a local copy
		servers: make(map[string]*server, len(ac.Servers)),
		backend: b,
	}
	g.storeMainlog(l)

	if ac.LogLevel != "" {
		g.mainlog().SetLevel(ac.LogLevel)
	}

	g.state = GuerrillaStateNew
	err := g.makeServers()

	// subscribe for any events that may come in while running
	g.subscribeEvents()
	return g, err
}

// Instantiate servers
func (g *guerrilla) makeServers() error {
	g.mainlog().Debug("making servers")
	var errs Errors
	for _, sc := range g.Config.Servers {
		if _, ok := g.servers[sc.ListenInterface]; ok {
			// server already instantiated
			continue
		}
		if errs := sc.Validate(); errs != nil {
			g.mainlog().WithError(errs).Errorf("Failed to create server [%s]", sc.ListenInterface)
			errs = append(errs, errs...)
			continue
		} else {
			server, err := newServer(&sc, g.backend, g.mainlog())
			if err != nil {
				g.mainlog().WithError(err).Errorf("Failed to create server [%s]", sc.ListenInterface)
				errs = append(errs, err)
			}
			if server != nil {
				g.servers[sc.ListenInterface] = server
				server.setAllowedHosts(g.Config.AllowedHosts)
			}
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

// find a server by interface, retuning the server or err
func (g *guerrilla) findServer(iface string) (*server, error) {
	g.guard.Lock()
	defer g.guard.Unlock()
	if server, ok := g.servers[iface]; ok {
		return server, nil
	}
	return nil, errors.New("server not found in g.servers")
}

func (g *guerrilla) removeServer(iface string) {
	g.guard.Lock()
	defer g.guard.Unlock()
	delete(g.servers, iface)
}

// setConfig sets the app config
func (g *guerrilla) setConfig(c *AppConfig) {
	g.guard.Lock()
	defer g.guard.Unlock()
	g.Config = *c
}

// setServerConfig config updates the server's config, which will update for the next connected client
func (g *guerrilla) setServerConfig(sc *ServerConfig) {
	g.guard.Lock()
	defer g.guard.Unlock()
	if _, ok := g.servers[sc.ListenInterface]; ok {
		g.servers[sc.ListenInterface].setConfig(sc)
	}
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

// subscribeEvents subscribes event handlers for configuration change events
func (g *guerrilla) subscribeEvents() {

	// main config changed
	g.Subscribe(EventConfigNewConfig, func(c *AppConfig) {
		g.setConfig(c)
	})

	// allowed_hosts changed, set for all servers
	g.Subscribe(EventConfigAllowedHosts, func(c *AppConfig) {
		g.mapServers(func(server *server) {
			server.setAllowedHosts(c.AllowedHosts)
		})
		g.mainlog().Infof("allowed_hosts config changed, a new list was set")
	})

	// the main log file changed
	g.Subscribe(EventConfigLogFile, func(c *AppConfig) {
		var err error
		var l log.Logger
		if l, err = log.GetLogger(c.LogFile); err == nil {
			g.storeMainlog(l)
			g.mapServers(func(server *server) {
				// it will change server's logger when the next client gets accepted
				server.mainlogStore.Store(l)
			})
			g.mainlog().Infof("main log for new clients changed to to [%s]", c.LogFile)
		} else {
			g.mainlog().WithError(err).Errorf("main logging change failed [%s]", c.LogFile)
		}

	})

	// re-open the main log file (file not changed)
	g.Subscribe(EventConfigLogReopen, func(c *AppConfig) {
		g.mainlog().Reopen()
		g.mainlog().Infof("re-opened main log file [%s]", c.LogFile)
	})

	// when log level changes, apply to mainlog and server logs
	g.Subscribe(EventConfigLogLevel, func(c *AppConfig) {
		g.mainlog().SetLevel(c.LogLevel)
		g.mapServers(func(server *server) {
			server.log.SetLevel(c.LogLevel)
		})
		g.mainlog().Infof("log level changed to [%s]", c.LogLevel)
	})

	// server config was updated
	g.Subscribe(EventConfigServerConfig, func(sc *ServerConfig) {
		g.setServerConfig(sc)
	})

	// add a new server to the config & start
	g.Subscribe(EventConfigEvServerNew, func(sc *ServerConfig) {
		if _, err := g.findServer(sc.ListenInterface); err != nil {
			// not found, lets add it
			if err := g.makeServers(); err != nil {
				g.mainlog().WithError(err).Error("cannot add server [%s]", sc.ListenInterface)
				return
			}
			g.mainlog().Infof("New server added [%s]", sc.ListenInterface)
			if g.state == GuerrillaStateStarted {
				err := g.Start()
				if err != nil {
					g.mainlog().WithError(err).Info("Event server_change:new_server returned errors when starting")
				}
			}
		}
	})
	// start a server that already exists in the config and has been enabled
	g.Subscribe(EventConfigServerStart, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if server.state == ServerStateStopped || server.state == ServerStateNew {
				g.mainlog().Infof("Starting server [%s]", server.listenInterface)
				err := g.Start()
				if err != nil {
					g.mainlog().WithError(err).Info("Event server_change:start_server returned errors when starting")
				}
			}
		}
	})
	// stop running a server
	g.Subscribe(EventConfigServerStop, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if server.state == ServerStateRunning {
				server.Shutdown()
				g.mainlog().Infof("Server [%s] stopped.", sc.ListenInterface)
			}
		}
	})
	// server was removed from config
	g.Subscribe(EventConfigServerRemove, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			server.Shutdown()
			g.removeServer(sc.ListenInterface)
			g.mainlog().Infof("Server [%s] removed from config, stopped it.", sc.ListenInterface)
		}
	})

	// TLS changes
	g.Subscribe(EventConfigServerTLSConfig, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if err := server.configureSSL(); err == nil {
				g.mainlog().Infof("Server [%s] new TLS configuration loaded", sc.ListenInterface)
			} else {
				g.mainlog().WithError(err).Errorf("Server [%s] failed to load the new TLS configuration", sc.ListenInterface)
			}
		}
	})
	// when server's timeout change.
	g.Subscribe(EventConfigServerTimeout, func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			server.setTimeout(sc.Timeout)
		})
	})
	// when server's max clients change.
	g.Subscribe(EventConfigServerMaxClients, func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			// TODO resize the pool somehow
		})
	})
	// when a server's log file changes
	g.Subscribe(EventConfigServerLogFile, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			var err error
			var l log.Logger
			if l, err = log.GetLogger(sc.LogFile); err == nil {
				g.storeMainlog(l)
				backends.Svc.StoreMainlog(l)
				// it will change to the new logger on the next accepted client
				server.logStore.Store(l)

				g.mainlog().Infof("Server [%s] changed, new clients will log to: [%s]",
					sc.ListenInterface,
					sc.LogFile,
				)
			} else {
				g.mainlog().WithError(err).Errorf(
					"Server [%s] log change failed to: [%s]",
					sc.ListenInterface,
					sc.LogFile,
				)
			}
		}
	})
	// when the daemon caught a sighup, event for individual server
	g.Subscribe(EventConfigServerLogReopen, func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			server.log.Reopen()
			g.mainlog().Infof("Server [%s] re-opened log file [%s]", sc.ListenInterface, sc.LogFile)
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
		g.mainlog().Infof("Starting: %s", ListenInterface)
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
			g.mainlog().Infof("shutdown completed for [%s]", ListenInterface)
		}
	}
	if err := g.backend.Shutdown(); err != nil {
		g.mainlog().WithError(err).Warn("Backend failed to shutdown")
	} else {
		g.mainlog().Infof("Backend shutdown completed")
	}
}

// SetLogger sets the logger for the app and propagates it to sub-packages (eg.
func (g *guerrilla) SetLogger(l log.Logger) {
	l.SetLevel(g.Config.LogLevel)
	g.storeMainlog(l)
	backends.Svc.StoreMainlog(l)
}
