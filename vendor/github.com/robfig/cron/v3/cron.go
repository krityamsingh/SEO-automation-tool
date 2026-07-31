package cron

import (
	"sync"
)

type EntryID int

type Cron struct {
	mu      sync.Mutex
	funcs   []func()
	stop    chan struct{}
	running bool
}

func New() *Cron {
	return &Cron{
		stop: make(chan struct{}),
	}
}

func (c *Cron) AddFunc(spec string, cmd func()) (EntryID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.funcs = append(c.funcs, cmd)
	return EntryID(len(c.funcs)), nil
}

func (c *Cron) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()
}

func (c *Cron) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.running = false
	}
}
