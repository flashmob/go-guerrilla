package guerrilla

import (
	"errors"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPoolShuttingDown = errors.New("server pool: shutting down")
)

// a struct can be pooled if it has the following interface
type Poolable interface {
	// ability to set read/write timeout
	setTimeout(t time.Duration)
	// set a new connection and client id
	init(c net.Conn, clientID uint64, ep *mail.Pool)
	// get a unique id
	getID() uint64
}

// Pool holds Clients.
type Pool struct {
	// clients that are ready to be borrowed
	pool chan Poolable
	// semaphore to control number of maximum borrowed clients
	sem chan bool
	// book-keeping of clients that have been lent
	activeClients     lentClients
	isShuttingDownFlg atomic.Value
	poolGuard         sync.Mutex
	ShutdownChan      chan int
}

type lentClients struct {
	m  map[uint64]Poolable
	mu sync.Mutex // guards access to this struct
	wg sync.WaitGroup
}

// maps the callback on all lentClients
func (c *lentClients) mapAll(callback func(p Poolable)) {
	defer c.mu.Unlock()
	c.mu.Lock()
	for _, item := range c.m {
		callback(item)
	}
}

// operation performs an operation on a Poolable item using the callback
func (c *lentClients) operation(callback func(p Poolable), item Poolable) {
	defer c.mu.Unlock()
	c.mu.Lock()
	callback(item)
}

// NewPool creates a new pool of Clients.
func NewPool(poolSize int) *Pool {
	return &Pool{
		pool:          make(chan Poolable, poolSize),
		sem:           make(chan bool, poolSize),
		activeClients: lentClients{m: make(map[uint64]Poolable, poolSize)},
		ShutdownChan:  make(chan int, 1),
	}
}
func (p *Pool) Start() {
	p.isShuttingDownFlg.Store(true)
}

// Lock the pool from borrowing then remove all active clients
// each active client's timeout is lowered to 1 sec and notified
// to stop accepting commands
func (p *Pool) ShutdownState() {
	const aVeryLowTimeout = 1
	p.poolGuard.Lock() // ensure no other thread is in the borrowing now
	defer p.poolGuard.Unlock()
	p.isShuttingDownFlg.Store(true) // no more borrowing
	p.ShutdownChan <- 1             // release any waiting p.sem

	// set a low timeout
	p.activeClients.mapAll(func(p Poolable) {
		p.setTimeout(time.Duration(int64(aVeryLowTimeout)))
	})

}

func (p *Pool) ShutdownWait() {
	p.poolGuard.Lock() // ensure no other thread is in the borrowing now
	defer p.poolGuard.Unlock()
	p.activeClients.wg.Wait() // wait for clients to finish
	if len(p.ShutdownChan) > 0 {
		// drain
		<-p.ShutdownChan
	}
	p.isShuttingDownFlg.Store(false)
}

// returns true if the pool is shutting down
func (p *Pool) IsShuttingDown() bool {
	if value, ok := p.isShuttingDownFlg.Load().(bool); ok {
		return value
	}
	return false
}

// set a timeout for all lent clients
func (p *Pool) SetTimeout(duration time.Duration) {
	p.activeClients.mapAll(func(p Poolable) {
		p.setTimeout(duration)
	})
}

// Gets the number of active clients that are currently
// out of the pool and busy serving
func (p *Pool) GetActiveClientsCount() int {
	return len(p.sem)
}

// Borrow a Client from the pool. Will block if len(activeClients) > maxClients
func (p *Pool) Borrow(conn net.Conn, clientID uint64, logger log.Logger, ep *mail.Pool) (Poolable, error) {
	p.poolGuard.Lock()
	defer p.poolGuard.Unlock()

	var c Poolable
	if yes, really := p.isShuttingDownFlg.Load().(bool); yes && really {
		// pool is shutting down.
		return c, ErrPoolShuttingDown
	}
	select {
	case p.sem <- true: // block the client from serving until there is room
		select {
		case c = <-p.pool:
			c.init(conn, clientID, ep)
		default:
			c = NewClient(conn, clientID, logger, ep)
		}
		p.activeClientsAdd(c)

	case <-p.ShutdownChan: // unblock p.sem when shutting down
		// pool is shutting down.
		return c, ErrPoolShuttingDown
	}
	return c, nil
}

// Return returns a Client back to the pool.
func (p *Pool) Return(c Poolable) {
	p.activeClientsRemove(c)
	select {
	case p.pool <- c:
	default:
		// hasta la vista, baby...
	}

	<-p.sem // make room for the next serving client
}

func (p *Pool) activeClientsAdd(c Poolable) {
	p.activeClients.operation(func(item Poolable) {
		p.activeClients.wg.Add(1)
		p.activeClients.m[c.getID()] = item
	}, c)
}

func (p *Pool) activeClientsRemove(c Poolable) {
	p.activeClients.operation(func(item Poolable) {
		delete(p.activeClients.m, item.getID())
		p.activeClients.wg.Done()
	}, c)
}
