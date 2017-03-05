package guerrilla

import (
	"errors"
	"fmt"
	_ "fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"io/ioutil"
	"os"
)

type SMTP struct {
	config  *AppConfig
	logger  log.Logger
	backend backends.Backend
	g       Guerrilla
}

const defaultInterface = "127.0.0.1:2525"

// configureDefaults fills in default server settings for values that were not configured
func (s *SMTP) configureDefaults() error {
	if s.config.LogFile == "" {
		s.config.LogFile = log.OutputStderr.String()
	}
	if s.config.LogLevel == "" {
		s.config.LogLevel = "debug"
	}
	if len(s.config.AllowedHosts) == 0 {
		if h, err := os.Hostname(); err != nil {
			return err
		} else {
			s.config.AllowedHosts = append(s.config.AllowedHosts, h)
		}
	}
	h, err := os.Hostname()
	if err != nil {
		return err
	}
	if len(s.config.Servers) == 0 {
		sc := ServerConfig{}
		sc.LogFile = s.config.LogFile
		sc.ListenInterface = defaultInterface
		sc.IsEnabled = true
		sc.Hostname = h
		sc.MaxClients = 100
		sc.Timeout = 30
		sc.MaxSize = 10 << 20 // 10 Mebibytes
		s.config.Servers = append(s.config.Servers, sc)
	} else {
		// make sure each server has defaults correctly configured
		for i := range s.config.Servers {
			if s.config.Servers[i].Hostname == "" {
				s.config.Servers[i].Hostname = h
			}
			if s.config.Servers[i].MaxClients == 0 {
				s.config.Servers[i].MaxClients = 100
			}
			if s.config.Servers[i].Timeout == 0 {
				s.config.Servers[i].Timeout = 20
			}
			if s.config.Servers[i].MaxSize == 0 {
				s.config.Servers[i].MaxSize = 10 << 20 // 10 Mebibytes
			}
			if s.config.Servers[i].ListenInterface == "" {
				return errors.New(fmt.Sprintf("Listen interface not specified for server at index %d", i))
			}
			if s.config.Servers[i].LogFile == "" {
				s.config.Servers[i].LogFile = s.config.LogFile
			}
			// validate the server config
			err = s.config.Servers[i].Validate()
			if err != nil {
				return err
			}
		}

	}
	return nil

}

func (s *SMTP) configureDefaultBackend() error {
	h, err := os.Hostname()
	if err != nil {
		return err
	}
	if len(s.config.BackendConfig) == 0 {
		bcfg := backends.BackendConfig{
			"log_received_mails": true,
			"save_workers_size":  1,
			"process_stack":      "HeadersParser|Header|Debugger",
			"primary_mail_host":  h,
		}
		s.backend, err = backends.New(bcfg, s.logger)
		if err != nil {
			return err
		}
	} else {
		if _, ok := s.config.BackendConfig["process_stack"]; !ok {
			s.config.BackendConfig["process_stack"] = "HeadersParser|Header|Debugger"
		}
		if _, ok := s.config.BackendConfig["primary_mail_host"]; !ok {
			s.config.BackendConfig["primary_mail_host"] = h
		}
		if _, ok := s.config.BackendConfig["save_workers_size"]; !ok {
			s.config.BackendConfig["save_workers_size"] = 1
		}

		if _, ok := s.config.BackendConfig["log_received_mails"]; !ok {
			s.config.BackendConfig["log_received_mails"] = false
		}
		s.backend, err = backends.New(s.config.BackendConfig, s.logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SMTP) Start() (err error) {
	if s.g == nil {
		if s.config == nil {
			s.config = &AppConfig{}
		}
		err = s.configureDefaults()
		if err != nil {
			return err
		}

		if s.logger == nil {
			s.logger, err = log.GetLogger(s.config.LogFile)
			if err != nil {
				return err
			}
		}
		if s.backend == nil {
			err = s.configureDefaultBackend()
			if err != nil {
				return
			}
		}
		s.g, err = New(s.config, s.backend, s.logger)
		if err != nil {
			return err
		}

	}
	return s.g.Start()
}

func (s *SMTP) Shutdown() {
	s.g.Shutdown()
}

// ReadConfig reads in the config from a json file.
func (s *SMTP) ReadConfig(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Could not read config file: %s", err.Error())
	}
	if s.config == nil {
		s.config = &AppConfig{}
	}
	if err := s.config.Load(data); err != nil {
		return err
	}
	return nil
}
