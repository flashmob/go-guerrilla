package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"time"
)

// ----------------------------------------------------------------------------------
// Processor Name: debugger
// ----------------------------------------------------------------------------------
// Description   : Log received emails
// ----------------------------------------------------------------------------------
// Config Options: log_reads bool - log if true
//               : sleep_seconds - how many seconds to pause for, useful to force a
//               : timeout. If sleep_seconds is 1 then a panic will be induced
// --------------:-------------------------------------------------------------------
// Input         : email envelope
// ----------------------------------------------------------------------------------
// Output        : none (only output to the log if enabled)
// ----------------------------------------------------------------------------------

func init() {
	Streamers["debug"] = func() *StreamDecorator {
		return StreamDebug()
	}
}

type streamDebuggerConfig struct {
	LogReads bool `json:"log_reads"`
	SleepSec int  `json:"sleep_seconds,omitempty"`
}

func StreamDebug() *StreamDecorator {
	sd := &StreamDecorator{}
	var config streamDebuggerConfig
	sd.Configure = func(cfg ConfigGroup) error {
		return sd.ExtractConfig(cfg, &config)
	}
	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			sd.Open = func(e *mail.Envelope) error {
				return nil
			}
			return StreamProcessWith(func(p []byte) (int, error) {
				str := string(p)
				if config.LogReads {
					fmt.Print(str)
					Log().WithField("p", string(p)).Info("Debug stream")
				}
				if config.SleepSec > 0 {
					Log().Infof("sleeping for %d", config.SleepSec)
					time.Sleep(time.Second * time.Duration(config.SleepSec))
					Log().Infof("woke up")

					if config.SleepSec == 1 {
						panic("panic on purpose")
					}
				}
				return sp.Write(p)
			})
		}
	return sd
}
