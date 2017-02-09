package backends

type loggerConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

// putting all the paces we need together
type LoggerBackend struct {
	config dummyConfig
}
