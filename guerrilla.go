package guerrilla

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"sync"
)

type Guerrilla interface {
	Start() (startErrors []error)
	Shutdown()
}

type guerrilla struct {
	Config  *AppConfig
	servers map[string]*server
	backend *Backend
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b *Backend) (Guerrilla, error) {
	g := &guerrilla{ac, make(map[string]*server, len(ac.Servers)), b}
	// Instantiate servers
	for _, sc := range ac.Servers {
		if !sc.IsEnabled {
			continue
		}
		// Add relevant app-wide config options to each server
		sc.AllowedHosts = ac.AllowedHosts
		server, err := newServer(sc, b)
		if err != nil {
			log.WithError(err).Error("Failed to create server")
		} else {
			// all good.
			g.servers[sc.ListenInterface] = server
		}
	}
	if len(g.servers) == 0 {
		return g, errors.New("There are no servers that can start, please check your config")
	}
	return g, nil
}

// Entry point for the application. Starts all servers.
func (g *guerrilla) Start() (startErrors []error) {
	if len(g.servers) == 0 {
		return append(startErrors, errors.New("No servers to start, please check the config"))
	}
	// channel for reading errors
	errs := make(chan error, len(g.servers))
	var startWG sync.WaitGroup
	startWG.Add(len(g.servers))
	// start servers, send any errors back to errs channel
	for ListenInterface := range g.servers {
		go func(s *server) {
			if err := s.Start(&startWG); err != nil {
				errs <- err
				startWG.Done()
			}
		}(g.servers[ListenInterface])
	}
	// wait for all servers to start
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
	for ListenInterface, s := range g.servers {
		s.Shutdown()
		log.Infof("shutdown completed for [%s]", ListenInterface)
	}
	log.Infof("Backend shutdown completed")
}
