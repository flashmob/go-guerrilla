package backends

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

type ConfigGroup map[string]interface{}

type BackendConfig map[ConfigSection]map[string]ConfigGroup

const (
	validateRcptTimeout = time.Second * 5
	defaultProcessor    = "Debugger"

	// streamBufferSize sets the size of the buffer for the streaming processors,
	// can be configured using `stream_buffer_size`
	configStreamBufferSize       = 4096
	configSaveWorkersCount       = 1
	configValidateWorkersCount   = 1
	configStreamWorkersCount     = 1
	configBackgroundWorkersCount = 1
	configSaveProcessSize        = 64
	configValidateProcessSize    = 64
	// configTimeoutSave: default timeout for saving email, if 'save_timeout' not present in config
	configTimeoutSave = time.Second * 30
	// configTimeoutValidateRcpt default timeout for validating rcpt to, if 'val_rcpt_timeout' not present in config
	configTimeoutValidateRcpt = time.Second * 5
	configTimeoutStream       = time.Second * 30
	configSaveStreamSize      = 64
	configPostProcessSize     = 64
)

func (c *BackendConfig) SetValue(section ConfigSection, name string, key string, value interface{}) {
	if *c == nil {
		*c = make(BackendConfig, 0)
	}
	if (*c)[section] == nil {
		(*c)[section] = make(map[string]ConfigGroup)
	}
	if (*c)[section][name] == nil {
		(*c)[section][name] = make(ConfigGroup)
	}
	(*c)[section][name][key] = value
}

func (c *BackendConfig) GetValue(section ConfigSection, name string, key string) interface{} {
	if (*c)[section] == nil {
		return nil
	}
	if (*c)[section][name] == nil {
		return nil
	}
	if v, ok := (*c)[section][name][key]; ok {
		return &v
	}
	return nil
}

// toLower normalizes the backendconfig lowercases the config's keys
func (c BackendConfig) toLower() {
	for section, v := range c {
		for k2, v2 := range v {
			if k2_lower := strings.ToLower(k2); k2 != k2_lower {
				c[section][k2_lower] = v2
				delete(c[section], k2) // delete the non-lowercased key
			}
		}
	}
}

func (c BackendConfig) lookupGroup(section ConfigSection, name string) ConfigGroup {
	if v, ok := c[section][name]; ok {
		return v
	}
	return nil
}

// ConfigureDefaults sets default values for the backend config,
// if no backend config was added before starting, then use a default config
// otherwise, see what required values were missed in the config and add any missing with defaults
func (c *BackendConfig) ConfigureDefaults() error {
	// set the defaults if no value has been configured
	// (always use lowercase)
	if c.GetValue(ConfigGateways, "default", "save_workers_size") == nil {
		c.SetValue(ConfigGateways, "default", "save_workers_size", 1)
	}
	if c.GetValue(ConfigGateways, "default", "save_process") == nil {
		c.SetValue(ConfigGateways, "default", "save_process", "HeadersParser|Header|Debugger")
	}
	if c.GetValue(ConfigProcessors, "header", "primary_mail_host") == nil {
		h, err := os.Hostname()
		if err != nil {
			return err
		}
		c.SetValue(ConfigProcessors, "header", "primary_mail_host", h)
	}
	if c.GetValue(ConfigProcessors, "debugger", "log_received_mails") == nil {
		c.SetValue(ConfigProcessors, "debugger", "log_received_mails", true)
	}
	return nil
}

// UnmarshalJSON custom handling of the ConfigSection keys (they're enumerated)
func (c *BackendConfig) UnmarshalJSON(b []byte) error {
	temp := make(map[string]map[string]ConfigGroup)
	err := json.Unmarshal(b, &temp)
	if err != nil {
		return err
	}
	if *c == nil {
		*c = make(BackendConfig)
	}
	for key, val := range temp {
		// map the key to a ConfigSection type
		var section ConfigSection
		if err := json.Unmarshal([]byte("\""+key+"\""), &section); err != nil {
			return err
		}
		if (*c)[section] == nil {
			(*c)[section] = make(map[string]ConfigGroup)
		}
		(*c)[section] = val
	}
	return nil

}

// MarshalJSON custom handling of ConfigSection keys (since JSON keys need to be strings)
func (c *BackendConfig) MarshalJSON() ([]byte, error) {
	temp := make(map[string]map[string]ConfigGroup)
	for key, val := range *c {
		// convert they key to a string
		temp[key.String()] = val
	}
	return json.Marshal(temp)
}

type ConfigSection int

const (
	ConfigProcessors ConfigSection = iota
	ConfigStreamProcessors
	ConfigGateways
)

func (o ConfigSection) String() string {
	switch o {
	case ConfigProcessors:
		return "processors"
	case ConfigStreamProcessors:
		return "stream_processors"
	case ConfigGateways:
		return "gateways"
	}
	return "unknown"
}

func (o *ConfigSection) UnmarshalJSON(b []byte) error {
	str := strings.Trim(string(b), `"`)
	str = strings.ToLower(str)
	switch {
	case str == "processors":
		*o = ConfigProcessors
	case str == "stream_processors":
		*o = ConfigStreamProcessors
	case str == "gateways":
		*o = ConfigGateways
	default:
		return errors.New("incorrect config section [" + str + "], may be processors, stream_processors or gateways")
	}
	return nil
}

func (o *ConfigSection) MarshalJSON() ([]byte, error) {
	ret := o.String()
	if ret == "unknown" {
		return []byte{}, errors.New("unknown config section")
	}
	return []byte(ret), nil
}

// All config structs extend from this
type BaseConfig interface{}

type stackConfigExpression struct {
	alias string
	name  string
}

func (e stackConfigExpression) String() string {
	if e.alias == e.name || e.alias == "" {
		return e.name
	}
	return fmt.Sprintf("%s:%s", e.alias, e.name)
}

type notFoundError func(s string) error

type stackConfig struct {
	list     []stackConfigExpression
	notFound notFoundError
}

type aliasMap map[string]string

// newAliasMap scans through the configured processors to produce a mapping
// alias -> processor name. This mapping is used to determine what configuration to use
// when making a new processor
func newAliasMap(cfg map[string]ConfigGroup) aliasMap {
	am := make(aliasMap, 0)
	for k, _ := range cfg {
		var alias, name string
		// format: <alias> : <processorName>
		if i := strings.Index(k, ":"); i > 0 && len(k) > i+2 {
			alias = k[0:i]
			name = k[i+1:]
		} else {
			alias = k
			name = k
		}
		am[strings.ToLower(alias)] = strings.ToLower(name)
	}
	return am
}

func NewStackConfig(config string, am aliasMap) (ret *stackConfig) {
	ret = new(stackConfig)
	cfg := strings.ToLower(strings.TrimSpace(config))
	if cfg == "" {
		return
	}
	items := strings.Split(cfg, "|")
	ret.list = make([]stackConfigExpression, len(items))
	pos := 0
	for i := range items {
		pos = len(items) - 1 - i // reverse order, since decorators are stacked
		ret.list[i] = stackConfigExpression{alias: items[pos], name: items[pos]}
		if processor, ok := am[items[pos]]; ok {
			ret.list[i].name = processor
		}
	}
	return ret
}

func newStackProcessorConfig(config string, am aliasMap) (ret *stackConfig) {
	ret = NewStackConfig(config, am)
	ret.notFound = func(s string) error {
		return errors.New(fmt.Sprintf("processor [%s] not found", s))
	}
	return ret
}

func newStackStreamProcessorConfig(config string, am aliasMap) (ret *stackConfig) {
	ret = NewStackConfig(config, am)
	ret.notFound = func(s string) error {
		return errors.New(fmt.Sprintf("stream processor [%s] not found", s))
	}
	return ret
}

// Changes returns a list of gateways whose config changed
func (c BackendConfig) Changes(oldConfig BackendConfig) (changed, added, removed map[string]bool) {
	// check the processors if changed
	changed = make(map[string]bool, 0)
	added = make(map[string]bool, 0)
	removed = make(map[string]bool, 0)
	cp := ConfigProcessors
	csp := ConfigStreamProcessors
	cg := ConfigGateways
	changedProcessors := changedConfigGroups(
		oldConfig[cp], c[cp])
	changedStreamProcessors := changedConfigGroups(
		oldConfig[csp], c[csp])
	configType := BaseConfig(&GatewayConfig{})
	aliasMapStream := newAliasMap(c[csp])
	aliasMapProcessor := newAliasMap(c[cp])
	// oldList keeps a track of gateways that have been compared for changes.
	// We remove the from the list when a gateway was processed
	// remaining items are assumed to be removed from the new config
	oldList := map[string]bool{}
	for g := range oldConfig[cg] {
		oldList[g] = true
	}
	// go through all the gateway configs,
	// make a list of all the ones that have processors whose config had changed
	for key, _ := range c[cg] {
		// check the processors in the SaveProcess and SaveStream settings to see if
		// they changed. If changed, then make a record of which gateways use the processors
		e, _ := Svc.ExtractConfig(ConfigGateways, key, c, configType)
		bcfg := e.(*GatewayConfig)
		config := NewStackConfig(bcfg.SaveProcess, aliasMapProcessor)
		for _, v := range config.list {
			if _, ok := changedProcessors[v.name]; ok {
				changed[key] = true
			}
		}

		config = NewStackConfig(bcfg.SaveStream, aliasMapStream)
		for _, v := range config.list {
			if _, ok := changedStreamProcessors[v.name]; ok {
				changed[key] = true
			}
		}
		if o, ok := oldConfig[cg][key]; ok {
			delete(oldList, key)
			if !reflect.DeepEqual(c[cg][key], o) {
				// whats changed
				changed[key] = true
			}
		} else {
			// whats been added
			added[key] = true
		}
	}
	// whats been removed
	for p := range oldList {
		removed[p] = true
	}
	return
}

func changedConfigGroups(old map[string]ConfigGroup, new map[string]ConfigGroup) map[string]bool {
	diff, added, removed := compareConfigGroup(old, new)
	var all []string
	all = append(all, diff...)
	all = append(all, removed...)
	all = append(all, added...)
	changed := make(map[string]bool, 0)
	for p := range all {
		changed[strings.ToLower(all[p])] = true
	}
	return changed
}

// compareConfigGroup compares two config groups
// returns a list of keys that changed, been added or removed to new
func compareConfigGroup(old map[string]ConfigGroup, new map[string]ConfigGroup) (diff, added, removed []string) {
	diff = make([]string, 0)
	added = make([]string, 0)
	removed = make([]string, 0)
	for p := range new {
		if o, ok := old[p]; ok {
			delete(old, p)
			if !reflect.DeepEqual(new[p], o) {
				// whats changed
				diff = append(diff, p)
			}
		} else {
			// whats been added
			added = append(added, p)
		}
	}
	// whats been removed
	for p := range old {
		removed = append(removed, p)
	}
	return
}

type GatewayConfig struct {
	// SaveWorkersCount controls how many concurrent workers to start. Defaults to 1
	SaveWorkersCount int `json:"save_workers_size,omitempty"`
	// ValidateWorkersCount controls how many concurrent recipient validation workers to start. Defaults to 1
	ValidateWorkersCount int `json:"validate_workers_size,omitempty"`
	// StreamWorkersCount controls how many concurrent stream workers to start. Defaults to 1
	StreamWorkersCount int `json:"stream_workers_size,omitempty"`
	// BackgroundWorkersCount controls how many concurrent background stream workers to start. Defaults to 1
	BackgroundWorkersCount int `json:"background_workers_size,omitempty"`

	// SaveProcess controls which processors to chain in a stack for saving email tasks
	SaveProcess string `json:"save_process,omitempty"`
	// SaveProcessSize limits the amount of messages waiting in the queue to get processed by SaveProcess
	SaveProcessSize int `json:"save_process_size,omitempty"`
	// ValidateProcess is like ProcessorStack, but for recipient validation tasks
	ValidateProcess string `json:"validate_process,omitempty"`
	// ValidateProcessSize limits the amount of messages waiting in the queue to get processed by ValidateProcess
	ValidateProcessSize int `json:"validate_process_size,omitempty"`

	// TimeoutSave is duration before timeout when saving an email, eg "29s"
	TimeoutSave string `json:"save_timeout,omitempty"`
	// TimeoutValidateRcpt duration before timeout when validating a recipient, eg "1s"
	TimeoutValidateRcpt string `json:"val_rcpt_timeout,omitempty"`
	// TimeoutStream duration before timeout when processing a stream eg "1s"
	TimeoutStream string `json:"stream_timeout,omitempty"`

	// StreamBufferLen controls the size of the output buffer, in bytes. Default is 4096
	StreamBufferSize int `json:"stream_buffer_size,omitempty"`
	// SaveStream is same as a SaveProcess, but uses the StreamProcessor stack instead
	SaveStream string `json:"save_stream,omitempty"`
	// SaveStreamSize limits the amount of messages waiting in the queue to get processed by SaveStream
	SaveStreamSize int `json:"save_stream_size,omitempty"`

	// PostProcessSize controls the length of thq queue for background processing
	PostProcessSize int `json:"post_process_size,omitempty"`
	// PostProcessProducer specifies which StreamProcessor to use for reading data to the post process
	PostProcessProducer string `json:"post_process_producer,omitempty"`
	// PostProcessConsumer is same as SaveStream, but controls
	PostProcessConsumer string `json:"post_process_consumer,omitempty"`
}

// saveWorkersCount gets the number of workers to use for saving email by reading the save_workers_size config value
// Returns 1 if no config value was set
func (c *GatewayConfig) saveWorkersCount() int {
	if c.SaveWorkersCount <= 0 {
		return configSaveWorkersCount
	}
	return c.SaveWorkersCount
}

func (c *GatewayConfig) validateWorkersCount() int {
	if c.ValidateWorkersCount <= 0 {
		return configValidateWorkersCount
	}
	return c.ValidateWorkersCount
}

func (c *GatewayConfig) streamWorkersCount() int {
	if c.StreamWorkersCount <= 0 {
		return configStreamWorkersCount
	}
	return c.StreamWorkersCount
}

func (c *GatewayConfig) backgroundWorkersCount() int {
	if c.BackgroundWorkersCount <= 0 {
		return configBackgroundWorkersCount
	}
	return c.BackgroundWorkersCount
}
func (c *GatewayConfig) saveProcessSize() int {
	if c.SaveProcessSize <= 0 {
		return configSaveProcessSize
	}
	return c.SaveProcessSize
}

func (c *GatewayConfig) validateProcessSize() int {
	if c.ValidateProcessSize <= 0 {
		return configValidateProcessSize
	}
	return c.ValidateProcessSize
}

func (c *GatewayConfig) saveStreamSize() int {
	if c.SaveStreamSize <= 0 {
		return configSaveStreamSize
	}
	return c.SaveStreamSize
}

func (c *GatewayConfig) postProcessSize() int {
	if c.PostProcessSize <= 0 {
		return configPostProcessSize
	}
	return c.PostProcessSize
}

// saveTimeout returns the maximum amount of seconds to wait before timing out a save processing task
func (gw *BackendGateway) saveTimeout() time.Duration {
	if gw.gwConfig.TimeoutSave == "" {
		return configTimeoutSave
	}
	t, err := time.ParseDuration(gw.gwConfig.TimeoutSave)
	if err != nil {
		return configTimeoutSave
	}
	return t
}

// validateRcptTimeout returns the maximum amount of seconds to wait before timing out a recipient validation  task
func (gw *BackendGateway) validateRcptTimeout() time.Duration {
	if gw.gwConfig.TimeoutValidateRcpt == "" {
		return configTimeoutValidateRcpt
	}
	t, err := time.ParseDuration(gw.gwConfig.TimeoutValidateRcpt)
	if err != nil {
		return configTimeoutValidateRcpt
	}
	return t
}
