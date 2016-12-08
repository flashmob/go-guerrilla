package server

import (
	"github.com/flashmob/go-guerrilla"
	"net"
	"errors"
	"sync/atomic"
	"time"
	"sync")

var (
	ErrPoolShuttingDown = errors.New("server pool: shutting down")
)

// Pool holds Clients.
type Pool struct {
	pool              chan *guerrilla.Client
	activeClients     chan *guerrilla.Client
	clients           allClients
	isShuttingDownFlg atomic.Value
	borrowGuard       sync.Mutex
}

type allClients struct {
	m    map[uint64]*guerrilla.Client
	mu   sync.Mutex // guards access to this struct
}


// NewPool creates a new pool of Clients.
func NewPool(poolSize int) *Pool {
	return &Pool{
		pool:   make(chan *guerrilla.Client, poolSize),
		activeClients: make(chan *guerrilla.Client, poolSize),
		clients: allClients{m:make(map[uint64]*guerrilla.Client, poolSize)},
	}
}

// Lock the pool from borrowing then remove all active clients
// each active client's timeout is lowered to 1 sec and notified
// to stop accepting commands
func (p *Pool) Shutdown() {
	const aVeryLowTimeout = 1
	p.borrowGuard.Lock() // lock indefinitely from borrowing from the pool
	p.isShuttingDownFlg.Store(true)
	var client *guerrilla.Client
	// remove active clients
	for ; ; {
		if len(p.activeClients) == 0 {
			// nothing to remove
			goto Done
		}
		client = <- p.activeClients
		client.SetTimeout(time.Duration(int64(aVeryLowTimeout)))
		killClient(client)
	}
	Done:
	close(p.activeClients)
}

// returns true if the pool is shutting down
func (p *Pool) IsShuttingDown() bool {
	return p.isShuttingDownFlg.Load().(bool)
}

// set a timeout for all clients
func (p *Pool) SetTimeout(duration time.Duration) {
	var client *guerrilla.Client
	p.clients.mu.Lock()
	defer p.clients.mu.Unlock()
	for _, client = range p.clients.m {
		client.SetTimeout(duration)
	}
}

// Gets the number of active clients that are currently
// out of the pool and busy serving
func (p *Pool) GetActiveClientsCount() int {
	return len(p.activeClients)
}

// Borrow a Client from the pool. Will block if len(activeClients) > maxClients
func (p *Pool) Borrow(conn net.Conn, clientID uint64) (*guerrilla.Client, error) {
	p.borrowGuard.Lock()
	defer p.borrowGuard.Unlock()
	var c *guerrilla.Client
	if yes, really := p.isShuttingDownFlg.Load().(bool); yes && really {
		// pool is shutting down.
		return c, ErrPoolShuttingDown
	}
	select {
	case c = <-p.pool:
		c.Reset(conn, clientID)
	default:
		c = guerrilla.NewClient(conn, clientID)
		p.clients.mu.Lock()
		p.clients.m[clientID] = c
		p.clients.mu.Unlock()
	}
	p.activeClients <- c // block the client from serving until there is room
	return c, nil
}

// Return returns a Client back to the pool.
func (p *Pool) Return(c *guerrilla.Client) {
	select {
	case p.pool <- c:
		c.ClearEmailData()
	default:
		// hasta la vista, baby...
		p.clients.mu.Lock()
		delete(p.clients.m, c.ClientID)
		p.clients.mu.Unlock()
	}
	<-p.activeClients // make room for the next serving client
}
