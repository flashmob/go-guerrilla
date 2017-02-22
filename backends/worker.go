package backends

import (
	"errors"
	"runtime/debug"
)

type Worker struct{}

func (w *Worker) workDispatcher(workIn chan *workerMsg, validateRcpt chan *workerMsg, p Processor, workerId int) {

	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			Log().Error("Recovered form panic:", r, string(debug.Stack()))
		}
		// close any connections / files
		Svc.shutdown()

	}()
	Log().Infof("Save mail worker started (#%d)", workerId)
	for {
		select {
		case msg := <-workIn:
			if msg == nil {
				Log().Debug("No more messages from saveMail")
				return
			}
			// process the email here
			// TODO we should check the err
			result, _ := p.Process(msg.mail, TaskSaveMail)
			if result.Code() < 300 {
				// if all good, let the gateway know that it was queued
				msg.notifyMe <- &notifyMsg{nil, msg.mail.QueuedId}
			} else {
				// notify the gateway about the error
				msg.notifyMe <- &notifyMsg{err: errors.New(result.String())}
			}
		case msg := <-validateRcpt:
			_, err := p.Process(msg.mail, TaskValidateRcpt)
			if err != nil {
				// validation failed
				msg.notifyMe <- &notifyMsg{err: err}
			} else {
				// all good.
				msg.notifyMe <- &notifyMsg{err: nil}
			}
		}

	}
}
