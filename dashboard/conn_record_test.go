package dashboard

import (
	"container/heap"
	"math/rand"
	"testing"
)

const (
	nConnRecords = 100
)

func TestConnRecord(t *testing.T) {
	var crHeap connRecordHeap = make([]connRecord, nConnRecords)
	var max connRecord
	for i := 0; i < nConnRecords; i++ {
		crHeap[i] = connRecord{rand.Int(), "abc"}
		if crHeap[i].count > max.count {
			max = crHeap[i]
		}
	}

	heap.Init(&crHeap)

	if max.count != heap.Pop(&crHeap).(connRecord).count {
		t.Error("Pop did not return maximum value")
	}
}
