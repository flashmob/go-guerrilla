package guerrilla

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
)

const (
	// all configured servers were just been created
	daemonStateNew = iota
	// ... been started and running
	daemonStateStarted
	// ... been stopped
	daemonStateStopped
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
	// guard controls access to g.servers
	guard sync.Mutex
	state int8
	EventHandler
	logStore
	backendStore
}

type logStore struct {
	atomic.Value
}

type backendStore struct {
	atomic.Value
}

type daemonEvent func(c *AppConfig)
type serverEvent func(sc *ServerConfig)

// Get loads the log.logger in an atomic operation. Returns a stderr logger if not able to load
func (ls *logStore) mainlog() log.Logger {
	if v, ok := ls.Load().(log.Logger); ok {
		return v
	}
	l, _ := log.GetLogger(log.OutputStderr.String(), log.InfoLevel.String())
	return l
}

// setMainlog stores the log value in an atomic operation
func (ls *logStore) setMainlog(log log.Logger) {
	ls.Store(log)
}

// Returns a new instance of Guerrilla with the given config, not yet running. Backend started.
func New(ac *AppConfig, b backends.Backend, l log.Logger) (Guerrilla, error) {
	g := &guerrilla{
		Config:  *ac, // take a local copy
		servers: make(map[string]*server, len(ac.Servers)),
	}
	g.backendStore.Store(b)
	g.setMainlog(l)

	if ac.LogLevel != "" {
		if h, ok := l.(*log.HookedLogger); ok {
			if h, err := log.GetLogger(h.GetLogDest(), ac.LogLevel); err == nil {
				g.setMainlog(h)
			}
		}
	}
	// Write the process id (pid) to a file
	// we should still be able to continue even if we can't write the pid, error will be logged by writePid()
	_ = g.writePid()

	g.state = daemonStateNew
	err := g.makeServers()
	if err != nil {
		return g, err
	}

	// start backend for processing email
	err = g.backend().Start()
	if err != nil {
		return g, err
	}

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
		if err := sc.Validate(); err != nil {
			g.mainlog().WithError(errs).Errorf("Failed to create server [%s]", sc.ListenInterface)
			errs = append(errs, err)
			continue
		} else {
			sc := sc // pin!
			server, err := newServer(&sc, g.backend(), g.mainlog())
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
		errs = append(errs, errors.New("there are no servers that can start, please check your config"))
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// findServer finds a server by iface (interface), retuning the server or err
func (g *guerrilla) findServer(iface string) (*server, error) {
	g.guard.Lock()
	defer g.guard.Unlock()
	if server, ok := g.servers[iface]; ok {
		return server, nil
	}
	return nil, errors.New("server not found in g.servers")
}

// removeServer removes a server from the list of servers
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

	events := map[Event]interface{}{}
	// main config changed
	events[EventConfigNewConfig] = daemonEvent(func(c *AppConfig) {
		g.setConfig(c)
	})
	// allowed_hosts changed, set for all servers
	events[EventConfigAllowedHosts] = daemonEvent(func(c *AppConfig) {
		g.mapServers(func(server *server) {
			server.setAllowedHosts(c.AllowedHosts)
		})
		g.mainlog().Infof("allowed_hosts config changed, a new list was set")
	})

	// the main log file changed
	events[EventConfigLogFile] = daemonEvent(func(c *AppConfig) {
		var err error
		var l log.Logger
		if l, err = log.GetLogger(c.LogFile, c.LogLevel); err == nil {
			g.setMainlog(l)
			g.mapServers(func(server *server) {
				// it will change server's logger when the next client gets accepted
				server.mainlogStore.Store(l)
			})
			g.mainlog().Infof("main log for new clients changed to [%s]", c.LogFile)
		} else {
			g.mainlog().WithError(err).Errorf("main logging change failed [%s]", c.LogFile)
		}

	})

	// re-open the main log file (file not changed)
	events[EventConfigLogReopen] = daemonEvent(func(c *AppConfig) {
		err := g.mainlog().Reopen()
		if err != nil {
			g.mainlog().WithError(err).Errorf("main log file [%s] failed to re-open", c.LogFile)
			return
		}
		g.mainlog().Infof("re-opened main log file [%s]", c.LogFile)
	})

	// when log level changes, apply to mainlog and server logs
	events[EventConfigLogLevel] = daemonEvent(func(c *AppConfig) {
		l, err := log.GetLogger(g.mainlog().GetLogDest(), c.LogLevel)
		if err == nil {
			g.logStore.Store(l)
			g.mapServers(func(server *server) {
				server.logStore.Store(l)
			})
			g.mainlog().Infof("log level changed to [%s]", c.LogLevel)
		}
	})

	// write out our pid whenever the file name changes in the config
	events[EventConfigPidFile] = daemonEvent(func(ac *AppConfig) {
		_ = g.writePid()
	})

	// server config was updated
	events[EventConfigServerConfig] = serverEvent(func(sc *ServerConfig) {
		g.setServerConfig(sc)
		g.mainlog().Infof("server %s config change event, a new config has been saved", sc.ListenInterface)
	})

	// add a new server to the config & start
	events[EventConfigServerNew] = serverEvent(func(sc *ServerConfig) {
		g.mainlog().Debugf("event fired [%s] %s", EventConfigServerNew, sc.ListenInterface)
		if _, err := g.findServer(sc.ListenInterface); err != nil {
			// not found, lets add it
			//
			if err := g.makeServers(); err != nil {
				g.mainlog().WithError(err).Errorf("cannot add server [%s]", sc.ListenInterface)
				return
			}
			g.mainlog().Infof("New server added [%s]", sc.ListenInterface)
			if g.state == daemonStateStarted {
				err := g.Start()
				if err != nil {
					g.mainlog().WithError(err).Info("Event server_change:new_server returned errors when starting")
				}
			}
		} else {
			g.mainlog().Debugf("new event, but server already fund")
		}
	})

	// start a server that already exists in the config and has been enabled
	events[EventConfigServerStart] = serverEvent(func(sc *ServerConfig) {
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
	events[EventConfigServerStop] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if server.state == ServerStateRunning {
				server.Shutdown()
				g.mainlog().Infof("Server [%s] stopped.", sc.ListenInterface)
			}
		}
	})

	// server was removed from config
	events[EventConfigServerRemove] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			server.Shutdown()
			g.removeServer(sc.ListenInterface)
			g.mainlog().Infof("Server [%s] removed from config, stopped it.", sc.ListenInterface)
		}
	})

	// TLS changes
	events[EventConfigServerTLSConfig] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if err := server.configureSSL(); err == nil {
				g.mainlog().Infof("Server [%s] new TLS configuration loaded", sc.ListenInterface)
			} else {
				g.mainlog().WithError(err).Errorf("Server [%s] failed to load the new TLS configuration", sc.ListenInterface)
			}
		}
	})
	// when server's timeout change.
	events[EventConfigServerTimeout] = serverEvent(func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			server.setTimeout(sc.Timeout)
		})
	})
	// when server's max clients change.
	events[EventConfigServerMaxClients] = serverEvent(func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			// TODO resize the pool somehow
		})
	})
	// when a server's log file changes
	events[EventConfigServerLogFile] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			var err error
			var l log.Logger
			level := g.mainlog().GetLevel()
			if l, err = log.GetLogger(sc.LogFile, level); err == nil {
				g.setMainlog(l)
				backends.Svc.SetMainlog(l)
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
	events[EventConfigServerLogReopen] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if err = server.log().Reopen(); err != nil {
				g.mainlog().WithError(err).Errorf("server [%s] log file [%s] failed to re-open", sc.ListenInterface, sc.LogFile)
				return
			}
			g.mainlog().Infof("Server [%s] re-opened log file [%s]", sc.ListenInterface, sc.LogFile)
		}
	})
	// when the backend changes
	events[EventConfigBackendConfig] = daemonEvent(func(appConfig *AppConfig) {
		logger, _ := log.GetLogger(appConfig.LogFile, appConfig.LogLevel)
		// shutdown the backend first.
		var err error
		if err = g.backend().Shutdown(); err != nil {
			logger.WithError(err).Warn("Backend failed to shutdown")
			return
		}
		// init a new backend, Revert to old backend config if it fails
		if newBackend, newErr := backends.New(appConfig.BackendConfig, logger); newErr != nil {
			logger.WithError(newErr).Error("Error while loading the backend")
			err = g.backend().Reinitialize()
			if err != nil {
				logger.WithError(err).Fatal("failed to revert to old backend config")
				return
			}
			err = g.backend().Start()
			if err != nil {
				logger.WithError(err).Fatal("failed to start backend with old config")
				return
			}
			logger.Info("reverted to old backend config")
		} else {
			// swap to the bew backend (assuming old backend was shutdown so it can be safely swapped)
			if err := newBackend.Start(); err != nil {
				logger.WithError(err).Error("backend could not start")
			}
			logger.Info("new backend started")
			g.storeBackend(newBackend)
		}
	})
	var err error
	for topic, fn := range events {
		switch f := fn.(type) {
		case daemonEvent:
			err = g.Subscribe(topic, f)
		case serverEvent:
			err = g.Subscribe(topic, f)
		}
		if err != nil {
			g.mainlog().WithError(err).Errorf("failed to subscribe on topic [%s]", topic)
			break
		}
	}

}

func (g *guerrilla) storeBackend(b backends.Backend) {
	g.backendStore.Store(b)
	g.mapServers(func(server *server) {
		server.setBackend(b)
	})
}

func (g *guerrilla) backend() backends.Backend {
	if b, ok := g.backendStore.Load().(backends.Backend); ok {
		return b
	}
	return nil
}

// Entry point for the application. Starts all servers.
func (g *guerrilla) Start() error {
	var startErrors Errors
	g.guard.Lock()
	defer func() {
		g.state = daemonStateStarted
		g.guard.Unlock()
	}()
	if len(g.servers) == 0 {
		return append(startErrors, errors.New("no servers to start, please check the config"))
	}
	if g.state == daemonStateStopped {
		// when a backend is shutdown, we need to re-initialize before it can be started again
		if err := g.backend().Reinitialize(); err != nil {
			startErrors = append(startErrors, err)
		}
		if err := g.backend().Start(); err != nil {
			startErrors = append(startErrors, err)
		}
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
			g.mainlog().Infof("Starting: %s", s.listenInterface)
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
	}
	return nil
}

func (g *guerrilla) Shutdown() {

	// shut down the servers first
	g.mapServers(func(s *server) {
		if s.state == ServerStateRunning {
			s.Shutdown()
			g.mainlog().Infof("shutdown completed for [%s]", s.listenInterface)
		}
	})

	g.guard.Lock()
	defer func() {
		g.state = daemonStateStopped
		defer g.guard.Unlock()
	}()
	if err := g.backend().Shutdown(); err != nil {
		g.mainlog().WithError(err).Warn("Backend failed to shutdown")
	} else {
		g.mainlog().Infof("Backend shutdown completed")
	}
}

// SetLogger sets the logger for the app and propagates it to sub-packages (eg.
func (g *guerrilla) SetLogger(l log.Logger) {
	g.setMainlog(l)
	backends.Svc.SetMainlog(l)
}

// writePid writes the pid (process id) to the file specified in the config.
// Won't write anything if no file specified
func (g *guerrilla) writePid() (err error) {
	var f *os.File
	defer func() {
		if f != nil {
			if closeErr := f.Close(); closeErr != nil {
				err = closeErr
			}
		}
		if err != nil {
			g.mainlog().WithError(err).Errorf("error while writing pidFile (%s)", g.Config.PidFile)
		}
	}()
	if len(g.Config.PidFile) > 0 {
		if f, err = os.Create(g.Config.PidFile); err != nil {
			return err
		}
		pid := os.Getpid()
		if _, err := f.WriteString(fmt.Sprintf("%d", pid)); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
		g.mainlog().Infof("pid_file (%s) written with pid:%v", g.Config.PidFile, pid)
	}
	return nil
}

// CheckFileLimit checks the number of files we can open (works on OS'es that support the ulimit command)
func CheckFileLimit(c *AppConfig) (bool, int, uint64) {
	fileLimit, err := getFileLimit()
	maxClients := 0
	if err != nil {
		// since we can't get the limit, return true to indicate the check passed
		return true, maxClients, fileLimit
	}
	if c.Servers == nil {
		// no servers have been configured, assuming default
		maxClients = defaultMaxClients
	} else {
		for _, s := range c.Servers {
			maxClients += s.MaxClients
		}
	}
	if uint64(maxClients) > fileLimit {
		return false, maxClients, fileLimit
	}
	return true, maxClients, fileLimit
}
