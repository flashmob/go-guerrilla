package guerrilla

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

	beGuard  sync.Mutex
	backends BackendContainer
}

type logStore struct {
	atomic.Value
}

type BackendContainer map[string]backends.Backend

type daemonEvent func(c *AppConfig)
type serverEvent func(sc *ServerConfig)
type backendEvent func(c *AppConfig, gateway string)

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

// makeConfiguredBackends makes backends from the config
func (g *guerrilla) makeConfiguredBackends(l log.Logger) ([]backends.Backend, error) {
	var list []backends.Backend
	config := g.Config.BackendConfig[backends.ConfigGateways]
	if len(config) == 0 {
		return list, errors.New("no backends configured")
	}
	list = make([]backends.Backend, 0)
	for name := range config {
		if b, err := backends.New(name, g.Config.BackendConfig, l); err != nil {
			return nil, err
		} else {
			list = append(list, b)
		}
	}
	return list, nil
}

// New creates a new Guerrilla instance configured with backends and a logger
// Returns a new instance of Guerrilla with the given config, not yet running. Backend started.
// b can be nil. If nil. then it will use the config to make the backends
func New(ac *AppConfig, l log.Logger, b ...backends.Backend) (Guerrilla, error) {
	g := &guerrilla{
		Config:  *ac, // take a local copy
		servers: make(map[string]*server, len(ac.Servers)),
	}
	if 0 == len(b) {
		var err error
		b, err = g.makeConfiguredBackends(l)
		if err != nil {
			return g, err
		}
	}
	if g.backends == nil {
		g.backends = make(BackendContainer)
	}
	for i := range b {
		if b[i] == nil {
			return g, errors.New("cannot use a nil backend")
		}
		g.storeBackend(b[i])
	}
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

	// start backends for processing email
	_, err = g.mapBackends(func(b backends.Backend) error {
		return b.Start()
	})

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
	for serverID, sc := range g.Config.Servers {
		if _, ok := g.servers[sc.ListenInterface]; ok {
			// server already instantiated
			continue
		}
		if err := sc.Validate(); err != nil {
			g.mainlog().Fields("error", errs, "iface", sc.ListenInterface).
				Error("failed to create server")
			errs = append(errs, err)
			continue
		} else {
			sc := sc // pin!
			server, err := newServer(&sc, g.backend(sc.Gateway), g.mainlog(), serverID)
			if err != nil {
				g.mainlog().Fields("error", err, "iface", sc.ListenInterface).
					Error("failed to create server")
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

type mapBackendErrors []error

func (e mapBackendErrors) Error() string {
	data := make([]string, len(e))
	for i, s := range e {
		data[i] = fmt.Sprint(s)
	}
	return strings.Join(data, ",")
}

func (g *guerrilla) mapBackends(callback func(backend backends.Backend) error) (BackendContainer, error) {
	defer g.beGuard.Unlock()
	g.beGuard.Lock()
	var e mapBackendErrors
	for name := range g.backends {
		if err := callback(g.backends[name]); err != nil {
			e = append(e, err)
		}
	}
	if len(e) == 0 {
		return g.backends, nil
	}
	return g.backends, e
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
			g.mainlog().Fields("serverID", server.serverID, "event", EventConfigAllowedHosts).
				Info("allowed_hosts config changed, a new list was set")
		})
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
			g.mainlog().Fields("file", c.LogFile).
				Info("main log for new clients changed")
		} else {
			g.mainlog().Fields("error", err, "file", c.LogFile).
				Error("main logging change failed")
		}

	})

	// re-open the main log file (file not changed)
	events[EventConfigLogReopen] = daemonEvent(func(c *AppConfig) {
		err := g.mainlog().Reopen()
		if err != nil {
			g.mainlog().Fields("error", err, "file", c.LogFile).
				Error("main log file failed to re-open")
			return
		}
		g.mainlog().Fields("file", c.LogFile).Info("re-opened main log file")
	})

	// when log level changes, apply to mainlog and server logs
	events[EventConfigLogLevel] = daemonEvent(func(c *AppConfig) {
		l, err := log.GetLogger(g.mainlog().GetLogDest(), c.LogLevel)
		if err == nil {
			g.logStore.Store(l)
			g.mapServers(func(server *server) {
				server.logStore.Store(l)
			})
			g.mainlog().Fields("level", c.LogLevel).Info("log level changed")
		}
	})

	// write out our pid whenever the file name changes in the config
	events[EventConfigPidFile] = daemonEvent(func(ac *AppConfig) {
		_ = g.writePid()
	})

	// server config was updated
	events[EventConfigServerConfig] = serverEvent(func(sc *ServerConfig) {
		g.setServerConfig(sc)
		g.mainlog().Fields("iface", sc.ListenInterface).
			Info("server config change event, a new config has been saved")
	})

	// add a new server to the config & start
	events[EventConfigServerNew] = serverEvent(func(sc *ServerConfig) {
		values := []interface{}{"iface", sc.ListenInterface, "event", EventConfigServerNew}
		g.mainlog().Fields(values...).
			Debug("event fired")
		if _, err := g.findServer(sc.ListenInterface); err != nil {

			// not found, lets add it
			if err := g.makeServers(); err != nil {
				g.mainlog().Fields(append(values, "error", err)...).
					Error("cannot add server")
				return
			}
			g.mainlog().Fields(values...).Info("new server added")
			if g.state == daemonStateStarted {
				err := g.Start()
				if err != nil {
					g.mainlog().Fields(append(values, "error", err)...).
						Error("new server errors when starting")
				}
			}
		} else {
			g.mainlog().Fields(values...).
				Debug("new event, but server already fund")
		}
	})

	// start a server that already exists in the config and has been enabled
	events[EventConfigServerStart] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			fields := []interface{}{
				"iface", server.listenInterface,
				"serverID", server.serverID,
				"event", EventConfigServerStart}
			if server.state == ServerStateStopped || server.state == ServerStateNew {
				g.mainlog().Fields(fields...).
					Info("starting server")
				err := g.Start()
				if err != nil {
					g.mainlog().Fields(append(fields, "error", err)...).
						Info("event server_change:start_server returned errors when starting")
				}
			}
		}
	})

	// stop running a server
	events[EventConfigServerStop] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			if server.state == ServerStateRunning {
				server.Shutdown()
				g.mainlog().Fields(
					"event", EventConfigServerStop,
					"server", sc.ListenInterface,
					"serverID", server.serverID).
					Info("server stopped.")
			}
		}
	})

	// server was removed from config
	events[EventConfigServerRemove] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			server.Shutdown()
			g.removeServer(sc.ListenInterface)
			g.mainlog().Fields(
				"event", EventConfigServerRemove,
				"server", sc.ListenInterface,
				"serverID", server.serverID).
				Info("server removed from config, stopped it")
		}
	})

	// TLS changes
	events[EventConfigServerTLSConfig] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			fields := []interface{}{
				"iface", server.listenInterface,
				"serverID", server.serverID,
				"event", EventConfigServerTLSConfig}
			if err := server.configureTLS(); err == nil {
				g.mainlog().Fields(fields...).Info("server new TLS configuration loaded")
			} else {
				g.mainlog().Fields(append(fields, "error", err)...).
					Error("Server failed to load the new TLS configuration")
			}
		}
	})
	// when server's timeout change.
	events[EventConfigServerTimeout] = serverEvent(func(sc *ServerConfig) {
		g.mapServers(func(server *server) {
			fields := []interface{}{
				"iface", server.listenInterface,
				"serverID", server.serverID,
				"event", EventConfigServerTimeout,
				"timeout", sc.Timeout,
			}
			server.setTimeout(sc.Timeout)
			g.mainlog().Fields(fields...).Info("server timeout set")
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
			fields := []interface{}{
				"iface", server.listenInterface,
				"serverID", server.serverID,
				"event", EventConfigServerLogFile,
				"file", sc.LogFile,
			}
			if l, err = log.GetLogger(sc.LogFile, level); err == nil {
				g.setMainlog(l)
				backends.Svc.SetMainlog(l)
				// it will change to the new logger on the next accepted client
				server.logStore.Store(l)
				g.mainlog().Fields(fields...).Info("server log changed",
					sc.ListenInterface,
					sc.LogFile,
				)
			} else {
				g.mainlog().Fields(append(fields, "error", err)...).Error(
					"server log change failed")
			}
		}
	})
	// when the daemon caught a sighup, event for individual server
	events[EventConfigServerLogReopen] = serverEvent(func(sc *ServerConfig) {
		if server, err := g.findServer(sc.ListenInterface); err == nil {
			fields := []interface{}{"file", sc.LogFile,
				"iface", sc.ListenInterface,
				"serverID", server.serverID,
				"file", sc.LogFile,
				"event", EventConfigServerLogReopen}
			if err = server.log().Reopen(); err != nil {
				g.mainlog().Fields(
					append(fields, "error", err)...).
					Error("server log file failed to re-open")
				return
			}
			g.mainlog().Fields(fields).Info("server re-opened log file")
		}
	})

	// when the server's gateway setting changed
	events[EventConfigServerGatewayConfig] = serverEvent(func(sc *ServerConfig) {
		b := g.backend(sc.Gateway)
		if b == nil {
			g.mainlog().Fields("gateway", sc.Gateway, "event", EventConfigServerGatewayConfig).
				Error("could not change to gateway, not configured")
			return
		}
		g.storeBackend(b)
	})

	revertIfError := func(err error, name string, logger log.Logger, g *guerrilla) {
		if err != nil {
			logger.Fields("error", err, "gateway", name, "event", EventConfigServerGatewayConfig).
				Error("cannot change gateway config, reverting to old config")
			err = g.backend(name).Reinitialize()
			if err != nil {
				logger.Fields("error", err, "gateway", name, "event", EventConfigServerGatewayConfig).
					Error("failed to revert to old gateway config")
				return
			}
			err = g.backend(name).Start()
			if err != nil {
				logger.Fields("error", err, "gateway", name, "event", EventConfigServerGatewayConfig).
					Error("failed to start gateway with old config")
				return
			}
			logger.Fields("gateway", name, "event", EventConfigServerGatewayConfig).
				Info("reverted to old gateway config")
		}
	}

	events[EventConfigBackendConfigChanged] = backendEvent(func(appConfig *AppConfig, name string) {
		logger, _ := log.GetLogger(appConfig.LogFile, appConfig.LogLevel)
		var err error
		// shutdown the backend first.
		if err = g.backend(name).Shutdown(); err != nil {
			logger.Fields("error", err, "gateway", name, "event", EventConfigBackendConfigChanged).
				Error("gateway failed to shutdown")
			return // we can't do anything then
		}
		if newBackend, newErr := backends.New(name, appConfig.BackendConfig, logger); newErr != nil {
			err = newErr
			revertIfError(newErr, name, logger, g) // revert to old backend
			return
		} else {
			if err = newBackend.Start(); err != nil {
				logger.Fields("error", err, "gateway", name, "event", EventConfigBackendConfigChanged).
					Error("gateway could not start")
				revertIfError(err, name, logger, g) // revert to old backend
				return
			} else {
				logger.Fields("gateway", name, "event", EventConfigBackendConfigChanged).
					Info("gateway with new config started")
				g.storeBackend(newBackend)
			}
		}
	})

	// a new gateway was added
	events[EventConfigBackendConfigAdded] = backendEvent(func(appConfig *AppConfig, name string) {
		logger, _ := log.GetLogger(appConfig.LogFile, appConfig.LogLevel)
		// shutdown any old backend first.
		if newBackend, newErr := backends.New(name, appConfig.BackendConfig, logger); newErr != nil {
			logger.Fields("error", newErr, "gateway", name, "event", EventConfigBackendConfigAdded).
				Error("cannot add new gateway")
		} else {
			// swap to the bew gateway (assuming old gateway was shutdown so it can be safely swapped)
			if err := newBackend.Start(); err != nil {
				logger.Fields("error", err, "gateway", name, "event", EventConfigBackendConfigAdded).
					Error("cannot start new gateway")
			}
			logger.Fields("gateway", name).Info("new gateway started")
			g.storeBackend(newBackend)
		}
	})

	// remove a gateway (shut it down)
	events[EventConfigBackendConfigRemoved] = backendEvent(func(appConfig *AppConfig, name string) {
		logger, _ := log.GetLogger(appConfig.LogFile, appConfig.LogLevel)
		// shutdown the backend first.
		var err error
		// revert
		defer revertIfError(err, name, logger, g)
		if err = g.backend(name).Shutdown(); err != nil {
			logger.Fields("error", err, "gateway", name, "event", EventConfigBackendConfigRemoved).
				Error("gateway failed to shutdown")
			return
		}
		g.removeBackend(g.backend(name))
		logger.Fields("gateway", name, "event", EventConfigBackendConfigRemoved).Info("gateway removed")
	})

	// subscribe all of the above events
	var err error
	for topic, fn := range events {
		err = g.Subscribe(topic, fn)
		if err != nil {
			g.mainlog().Fields("error", err, "event", topic).
				Error("failed to subscribe on topic")
			break
		}
	}
}

func (g *guerrilla) removeBackend(b backends.Backend) {
	g.beGuard.Lock()
	defer g.beGuard.Unlock()
	delete(g.backends, b.Name())

}

func (g *guerrilla) storeBackend(b backends.Backend) {
	g.beGuard.Lock()
	defer g.beGuard.Unlock()
	g.backends[b.Name()] = b
	g.mapServers(func(server *server) {
		sc := server.configStore.Load().(ServerConfig)
		if b.Name() == sc.Gateway {
			server.setBackend(b)
		}
	})
}

func (g *guerrilla) backend(name string) backends.Backend {
	g.beGuard.Lock()
	defer g.beGuard.Unlock()
	if b, ok := g.backends[name]; ok {
		return b
	}
	// if not found, return a random one
	for b := range g.backends {
		return g.backends[b]
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
		if _, err := g.mapBackends(func(b backends.Backend) error {
			if err := b.Reinitialize(); err != nil {
				return err
			}
			return b.Start()
		}); err != nil {
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
			g.mainlog().Fields("iface", s.listenInterface, "serverID", s.serverID).
				Info("starting server")
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
			g.mainlog().Fields("iface", s.listenInterface, "serverID", s.serverID).Info("shutdown completed")
		}
	})

	g.guard.Lock()
	defer func() {
		g.state = daemonStateStopped
		defer g.guard.Unlock()
	}()

	if _, err := g.mapBackends(func(b backends.Backend) error {
		return b.Shutdown()
	}); err != nil {
		fmt.Println(err)
		g.mainlog().Fields("error", err).Error("backend failed to shutdown")
	} else {
		g.mainlog().Info("backend shutdown completed")
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
			g.mainlog().Fields("error", err, "file", g.Config.PidFile).Error("error while writing pidFile")
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
		g.mainlog().Fields("file", g.Config.PidFile, "pid", pid).Info("pid_file written")
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
