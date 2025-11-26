package nexuscache

// A Getter loads data for a key.
// This is a callback function that gets invoked when cache misses occur
// to fetch the source data from the database or other backend.
type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc implements Getter interface.
// This is a functional interface pattern - it allows users to pass either:
// - A function directly as a parameter, or
// - A struct that implements the Getter interface
type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}
