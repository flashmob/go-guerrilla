package guerrilla

import log "github.com/Sirupsen/logrus"

type Guerrilla interface {
	Start()
	Shutdown()
}

type guerrilla struct {
	Config  *AppConfig
	servers []*server
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig) Guerrilla {
	g := &guerrilla{ac, []*server{}}

	// Instantiate servers
	for _, sc := range ac.Servers {
		if !sc.IsEnabled {
			continue
		}

		// Add relevant app-wide config options to each server
		sc.AllowedHosts = ac.AllowedHosts
		server, err := newServer(sc, ac.Backend)
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
	}
}
