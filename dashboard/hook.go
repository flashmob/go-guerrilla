package dashboard

import (
	log "github.com/sirupsen/logrus"
)

type logHook int

func (h logHook) Levels() []log.Level {
	return log.AllLevels
}

// Checks fired logs for information that is relevant to the dashboard
func (h logHook) Fire(e *log.Entry) error {
	event, ok := e.Data["event"].(string)
	if !ok {
		return nil
	}

	var helo, ip, domain string
	if event == "mailfrom" {
		helo, ok = e.Data["helo"].(string)
		if !ok {
			return nil
		}
		if len(helo) > 16 {
			helo = helo[:16]
		}
		ip, ok = e.Data["address"].(string)
		if !ok {
			return nil
		}
		domain, ok = e.Data["domain"].(string)
		if !ok {
			return nil
		}
	}

	switch event {
	case "connect":
		store.nClients.Add(1)
	case "mailfrom":
		store.newConns <- conn{
			domain: domain,
			helo:   helo,
			ip:     ip,
		}
	case "disconnect":
		store.nClients.Sub(1)
	}
	return nil
}
