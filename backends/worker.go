package backends

import (
	"errors"
	"fmt"
	"runtime/debug"
)

type Worker struct{}

func (w *Worker) saveMailWorker(saveMailChan chan *savePayload, p Processor, workerId int) {

	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			fmt.Println("Recovered in f", r, string(debug.Stack()))
			Log().Error("Recovered form panic:", r, string(debug.Stack()))
		}
		// close any connections / files
		Service.Shutdown()

	}()
	Log().Infof("Save mail worker started (#%d)", workerId)
	for {
		payload := <-saveMailChan
		if payload == nil {
			Log().Debug("No more saveMailChan payload")
			return
		}
		// process the email here
		result, _ := p.Process(payload.mail)
		// if all good
		if result.Code() < 300 {
			payload.savedNotify <- &saveStatus{nil, payload.mail.QueuedId}
		} else {
			payload.savedNotify <- &saveStatus{errors.New(result.String()), ""}
		}

	}
}
