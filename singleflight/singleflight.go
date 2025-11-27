package singleflight

import "sync"

type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) DoOnce(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		// If g.m[key] is not empty, another goroutine is already requesting this key,
		// so wait for that request to complete and return the same result
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)        // Lock before initiating request
	g.m[key] = c       // Add to map indicating this key is being requested
	g.mu.Unlock()

	c.val, c.err = fn() // Execute the request
	c.wg.Done()          // Request completed

	g.mu.Lock()
	delete(g.m, key)     // Delete the key since request is done
	g.mu.Unlock()

	return c.val, c.err
}
