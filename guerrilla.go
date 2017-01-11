package guerrilla

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla/backends"
	"sync"
)

const (
	// server has just been created
	GuerrillaStateNew = iota
	// Server has been started and is running
	GuerrillaStateStarted
	// Server has just been stopped
	GuerrillaStateStopped
)

type Guerrilla interface {
	Start() (startErrors []error)
	Shutdown()
}

type guerrilla struct {
	Config  AppConfig
	servers map[string]*server
	backend backends.Backend
	// guard controls access to g.servers
	guard sync.Mutex
	state int8
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b backends.Backend) (Guerrilla, error) {
	g := &guerrilla{
		Config:  *ac, // take a local copy
		servers: make(map[string]*server, len(ac.Servers)),
		backend: b,
	}
	g.state = GuerrillaStateNew
	err := g.makeNewServers()
	// subscribe for any events that may come in while running
	g.subscribeEvents()
	return g, err
}

// Instantiate servers
func (g *guerrilla) makeNewServers() error {
	for _, sc := range g.Config.Servers {
		if _, ok := g.servers[sc.ListenInterface]; ok {
			// server already instantiated
			continue
		}
		// Add relevant app-wide config options to each server
		sc.AllowedHosts = g.Config.AllowedHosts
		server, err := newServer(&sc, g.backend)
		if err != nil {
			log.WithError(err).Errorf("Failed to create server [%s]", sc.ListenInterface)
		} else {
			// all good.
			g.servers[sc.ListenInterface] = server
		}
	}
	if len(g.servers) == 0 {
		return errors.New("There are no servers that can start, please check your config")
	}
	return nil
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
	g.makeNewServers()
}

func (g *guerrilla) setConfig(i int, sc *ServerConfig) {
	g.guard.Lock()
	defer g.guard.Unlock()
	g.Config.Servers[i] = *sc
	g.servers[sc.ListenInterface].setConfig(sc)
}

func (g *guerrilla) subscribeEvents() {

	// add a new server to the config & start
	Bus.Subscribe("server_change:new_server", func(sc *ServerConfig) {
		if i, _ := g.findServer(sc.ListenInterface); i == -1 {
			// not found, lets add it
			g.addServer(sc)
			log.Infof("New server added [%s]", sc.ListenInterface)
			if g.state == GuerrillaStateStarted {
				g.Start()
			}
		}
	})
	// start a server that already exists in config and has been instantiated
	Bus.Subscribe("server_change:start_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			if server.state == ServerStateStopped {
				g.setConfig(i, sc)
				g.Start()
			}
		}
	})
	// stop running a server
	Bus.Subscribe("server_change:stop_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			if server.state == ServerStateRunning {
				server.Shutdown()
			}
		}
	})
	// server was removed from config
	Bus.Subscribe("server_change:remove_server", func(sc *ServerConfig) {
		if i, server := g.findServer(sc.ListenInterface); i != -1 {
			server.Shutdown()
			g.removeServer(i, sc.ListenInterface)
		}
	})
}

// Entry point for the application. Starts all servers.
func (g *guerrilla) Start() (startErrors []error) {
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
	return startErrors
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
			log.Infof("shutdown completed for [%s]", ListenInterface)
		}
	}
	if err := g.backend.Shutdown(); err != nil {
		log.WithError(err).Warn("Backend failed to shutdown")
	} else {
		log.Infof("Backend shutdown completed")
	}
}
