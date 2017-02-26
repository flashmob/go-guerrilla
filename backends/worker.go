package backends

import (
	"errors"
	"runtime/debug"
)

type Worker struct{}

func (w *Worker) workDispatcher(workIn chan *workerMsg, p Processor, workerId int) {

	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			Log().Error("worker recovered form panic:", r, string(debug.Stack()))
		}
		// close any connections / files
		Svc.shutdown()

	}()
	Log().Infof("processing worker started (#%d)", workerId)
	for {
		select {
		case msg := <-workIn:
			if msg == nil {
				Log().Debugf("worker stopped (#%d)", workerId)
				return
			}
			if msg.task == TaskSaveMail {
				// process the email here
				// TODO we should check the err
				result, _ := p.Process(msg.e, TaskSaveMail)
				if result.Code() < 300 {
					// if all good, let the gateway know that it was queued
					msg.notifyMe <- &notifyMsg{nil, msg.e.QueuedId}
				} else {
					// notify the gateway about the error
					msg.notifyMe <- &notifyMsg{err: errors.New(result.String())}
				}
			} else if msg.task == TaskValidateRcpt {
				_, err := p.Process(msg.e, TaskValidateRcpt)
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
}
