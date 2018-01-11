// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"sync"
	"time"
)

// Info stores information about connections made to a container.
type Info struct {
	// NumConnections holds the number of connections currently established
	// to a container.
	NumConnections int
	// LastConnection holds the time of the last connection.
	LastConnection time.Time
}

// NewInMemory creates and returns a new in memory store.
func NewInMemory() *InMemory {
	return &InMemory{
		db: make(map[string]*Info),
	}
}

// InMemory is a store which stores connection information in memory.
type InMemory struct {
	sync.RWMutex
	db map[string]*Info
}

// AddConn adds to the store a connection with the given id. Multiple
// connections can be added with the same id.
func (s *InMemory) AddConn(id string) error {
	s.Lock()
	defer s.Unlock()
	info := getInfo(s.db, id)
	info.NumConnections++
	info.LastConnection = time.Now()
	s.db[id] = info
	return nil
}

// RemoveConn removes the connection with the given id from the store.
func (s *InMemory) RemoveConn(id string) error {
	s.Lock()
	defer s.Unlock()
	info := getInfo(s.db, id)
	if info.NumConnections > 1 {
		info.NumConnections--
		info.LastConnection = time.Now()
		s.db[id] = info
		return nil
	}
	delete(s.db, id)
	return nil
}

// Info returns information about the connection with the given id.
func (s *InMemory) Info(id string) (*Info, error) {
	s.RLock()
	defer s.RUnlock()
	return getInfo(s.db, id), nil
}

func getInfo(db map[string]*Info, id string) *Info {
	info, ok := db[id]
	if ok {
		return info
	}
	return &Info{
		LastConnection: time.Now(),
	}
}
