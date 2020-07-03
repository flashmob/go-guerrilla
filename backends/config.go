package backends

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type ConfigGroup map[string]interface{}

type BackendConfig map[string]map[string]ConfigGroup

/*

TODO change to thus

type BackendConfig struct {
	Processors map[string]ConfigGroup `json:"processors,omitempty"`
	StreamProcessors map[string]ConfigGroup `json:"stream_processors,omitempty"`
	Gateways map[string]ConfigGroup `json:"gateways,omitempty"`
}

*/

func (c *BackendConfig) SetValue(ns configNameSpace, name string, key string, value interface{}) {
	nsKey := ns.String()
	if *c == nil {
		*c = make(BackendConfig, 0)
	}
	if (*c)[nsKey] == nil {
		(*c)[nsKey] = make(map[string]ConfigGroup)
	}
	if (*c)[nsKey][name] == nil {
		(*c)[nsKey][name] = make(ConfigGroup)
	}
	(*c)[nsKey][name][key] = value
}

func (c *BackendConfig) GetValue(ns configNameSpace, name string, key string) interface{} {
	nsKey := ns.String()
	if (*c)[nsKey] == nil {
		return nil
	}
	if (*c)[nsKey][name] == nil {
		return nil
	}
	if v, ok := (*c)[nsKey][name][key]; ok {
		return &v
	}
	return nil
}

// toLower normalizes the backendconfig lowercases the config's keys
func (c BackendConfig) toLower() {
	for k, v := range c {
		var l string
		if l = strings.ToLower(k); k != l {
			c[l] = v
			delete(c, k)
		}
		for k2, v2 := range v {
			if l2 := strings.ToLower(k2); k2 != l2 {
				c[l][l2] = v2
				delete(c[l], k)
			}
		}
	}
}

func (c BackendConfig) group(ns string, name string) ConfigGroup {
	if v, ok := c[ns][name]; ok {
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

type configNameSpace int

const (
	ConfigProcessors configNameSpace = iota
	ConfigStreamProcessors
	ConfigGateways
)

func (o configNameSpace) String() string {
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
	cp := ConfigProcessors.String()
	csp := ConfigStreamProcessors.String()
	cg := ConfigGateways.String()
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
		// check the processors in the SaveProcess and StreamSaveProcess settings to see if
		// they changed. If changed, then make a record of which gateways use the processors
		e, _ := Svc.ExtractConfig(ConfigGateways, key, c, configType)
		bcfg := e.(*GatewayConfig)
		config := NewStackConfig(bcfg.SaveProcess, aliasMapProcessor)
		for _, v := range config.list {
			if _, ok := changedProcessors[v.name]; ok {
				changed[key] = true
			}
		}

		config = NewStackConfig(bcfg.StreamSaveProcess, aliasMapStream)
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
