package guerrilla

import log "github.com/Sirupsen/logrus"

type Guerrilla struct {
	Config  *AppConfig
	servers []*Server
}

// Returns a new instance of Guerrilla with the given config, not yet running.
func New(ac *AppConfig) *Guerrilla {
	g := &Guerrilla{ac, []*Server{}}

	// Instantiate servers
	for _, sc := range ac.Servers {
		if !sc.IsEnabled {
			continue
		}

		// Add relevant app-wide config options to each server
		sc.AllowedHosts = ac.AllowedHosts
		server, err := NewServer(sc, ac.Backend)
		if err != nil {
			log.WithError(err).Error("Failed to create server")
		}
		g.servers = append(g.servers, server)
	}
	return g
}

// Entry point for the application. Starts all servers.
func (g *Guerrilla) Start() {
	for _, s := range g.servers {
		go s.Start()
	}
}
