package guerrilla

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPoolShuttingDown = errors.New("server pool: shutting down")
)

// Pool holds Clients.
type Pool struct {
	// clients that are ready to be borrowed
	pool chan *client
	// semaphore to control number of maximum borrowed clients
	sem chan bool
	// book-keeping of clients that have been lent
	lendingBook       lentClients
	isShuttingDownFlg atomic.Value
	borrowGuard       sync.Mutex
	ShutdownChan      chan int
}

type lentClients struct {
	m  map[uint64]*client
	mu sync.Mutex // guards access to this struct
}

// NewPool creates a new pool of Clients.
func NewPool(poolSize int) *Pool {
	return &Pool{
		pool:         make(chan *client, poolSize),
		sem:          make(chan bool, poolSize),
		lendingBook:  lentClients{m: make(map[uint64]*client, poolSize)},
		ShutdownChan: make(chan int, 1),
	}
}

// Lock the pool from borrowing then remove all active clients
// each active client's timeout is lowered to 1 sec and notified
// to stop accepting commands
func (p *Pool) Shutdown() {
	const aVeryLowTimeout = 1
	p.isShuttingDownFlg.Store(true) // close from borrowing
	p.ShutdownChan <- 1             // release any waiting p.sem
	p.borrowGuard.Lock()            // ensure no other thread is in the borrowing now
	// set a low timeout
	for _, c := range p.lendingBook.m {
		c.SetTimeout(time.Duration(int64(aVeryLowTimeout)))
	}
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
	var client *client
	p.lendingBook.mu.Lock()
	defer p.lendingBook.mu.Unlock()
	for _, client = range p.lendingBook.m {
		client.SetTimeout(duration)
	}
}

// Gets the number of active clients that are currently
// out of the pool and busy serving
func (p *Pool) GetActiveClientsCount() int {
	return len(p.sem)
}

// Borrow a Client from the pool. Will block if len(activeClients) > maxClients
func (p *Pool) Borrow(conn net.Conn, clientID uint64) (*client, error) {
	p.borrowGuard.Lock()
	defer p.borrowGuard.Unlock()
	var c *client
	if yes, really := p.isShuttingDownFlg.Load().(bool); yes && really {
		// pool is shutting down.
		return c, ErrPoolShuttingDown
	}
	select {
	case p.sem <- true: // block the client from serving until there is room
		select {
		case c = <-p.pool:
			c.Init(conn, clientID)
		default:
			c = NewClient(conn, clientID)
		}
		p.lendingBookAdd(c)

	case <-p.ShutdownChan:
		// pool is shutting down.
		return c, ErrPoolShuttingDown
	}
	return c, nil
}

// Return returns a Client back to the pool.
func (p *Pool) Return(c *client) {
	select {
	case p.pool <- c:
		c.reset()
	default:
		// hasta la vista, baby...
	}
	p.lendingBookRemove(c)
	<-p.sem // make room for the next serving client
}

func (p *Pool) lendingBookAdd(c *client) {
	p.lendingBook.mu.Lock()
	p.lendingBook.m[c.ID] = c
	p.lendingBook.mu.Unlock()
}

func (p *Pool) lendingBookRemove(c *client) {
	p.lendingBook.mu.Lock()
	delete(p.lendingBook.m, c.ID)
	p.lendingBook.mu.Unlock()
}
