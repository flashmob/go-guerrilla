package guerrilla

// TODO: Move backends to cmd. When running `guerrillad` you must specify backend
// name as a string, so this mapping is necessary, but when running through the
// guerrilla public API, you simply pass a backend object
var backends map[string]Backend

type Backend interface {
	Initialize(*BackendConfig) error
	Process(*Client)
}
