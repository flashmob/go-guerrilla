package guerrilla

import log "github.com/Sirupsen/logrus"

// Entry point for the application.
func Run(ac *AppConfig) {
	for _, sc := range ac.Servers {
		// Add app-wide allowed hosts to each server
		sc.AllowedHosts = ac.AllowedHosts
		server, err := NewServer(sc)
		if err != nil {
			log.WithError(err).Error("Failed to create server")
		}
		go server.run()
	}
}
