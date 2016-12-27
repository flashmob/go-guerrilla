package guerrilla

import (
	log "github.com/Sirupsen/logrus"
)

type Guerrilla interface {
	Start()
	Shutdown()
}

type guerrilla struct {
	Config  *AppConfig
	servers []*server
	backend Backend
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig, b *Backend) Guerrilla {
	g := &guerrilla{ac, []*server{}, *b}
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
		}
		g.servers = append(g.servers, server)
	}
	return g
}

// Entry point for the application. Starts all servers.
func (g *guerrilla) Start() {
	for _, s := range g.servers {
		go s.Start()
	}
}

func (g *guerrilla) Shutdown() {
	for _, s := range g.servers {
		s.Shutdown()
		log.Infof("shutdown completed for [%s]", s.config.ListenInterface)
	}
	g.backend.Shutdown()
	log.Infof("Backend shutdown completed")
}
