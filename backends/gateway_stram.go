package backends

import (
	"github.com/flashmob/go-guerrilla/log"
	"io"
)

type StreamBackendGateway struct {
	BackendGateway

	config *StreamBackendConfig

	pr *io.PipeReader
	pw *io.PipeWriter
}

type StreamBackendConfig struct {
	StreamSaveProcess string `json:"stream_save_process,omitempty"`
}

func NewStreamBackend(backendConfig BackendConfig, l log.Logger) (Backend, error) {
	b, err := New(backendConfig, l)
	if err != nil {
		return b, err
	}
	if bg, ok := b.(*BackendGateway); ok {
		sb := new(StreamBackendGateway)
		sb.BackendGateway = *bg
		return sb, nil
	}
	return b, err

}

func (gw *StreamBackendGateway) loadConfig(backendConfig BackendConfig) (err error) {
	configType := BaseConfig(&StreamBackendConfig{})
	bcfg, err := Svc.ExtractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*StreamBackendConfig)
	gw.config = m
	return nil
}
