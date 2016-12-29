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
	servers []server
	backend *Backend
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b *Backend) Guerrilla {
	g := &guerrilla{ac, []server{}, b}
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
			g.servers = append(g.servers, *server)
		}
	}
	return g
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
	for i := 0; i < len(g.servers); i++ {
		go func(s *server) {
			if err := s.Start(&startWG); err != nil {
				errs <- err
				startWG.Done()
			}
		}(&g.servers[i])
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
	for _, s := range g.servers {
		s.Shutdown()
		log.Infof("shutdown completed for [%s]", s.config.ListenInterface)
	}
	log.Infof("Backend shutdown completed")
}
