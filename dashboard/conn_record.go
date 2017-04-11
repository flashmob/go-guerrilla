package dashboard

import "container/heap"

// Records ranking of one unique connection in domain/ip/helo rankings.
type connRecord struct {
	// Number of records of this type
	count int
	// Name of the record, either the domain, IP, or helo
	value string
}

// Contains all ranking of a particular type (domain/ip/helo).
// Tracks ranking ordering by implementing heap.Interface
type connRecordHeap []connRecord

func (crh connRecordHeap) Len() int {
	return len(crh)
}

func (crh connRecordHeap) Less(i, j int) bool {
	return crh[i].count > crh[j].count
}

func (crh connRecordHeap) Swap(i, j int) {
	crh[i], crh[j] = crh[j], crh[i]
}

func (crh *connRecordHeap) Push(x interface{}) {
	*crh = append(*crh, x.(connRecord))
}

func (crh *connRecordHeap) Pop() interface{} {
	old := *crh
	l := len(old)
	toPop := old[l-1]
	*crh = old[:l-1]
	return toPop
}

// Gets N records with the greatest counts, maintaining the state of the heap
func (crh *connRecordHeap) GetN(n int) []connRecord {
	nHighest := make([]connRecord, n)
	for i := 0; i < n; i++ {
		nHighest[i] = heap.Pop(crh).(connRecord)
	}
	for _, cr := range nHighest {
		heap.Push(crh, cr)
	}
	return nHighest
}
