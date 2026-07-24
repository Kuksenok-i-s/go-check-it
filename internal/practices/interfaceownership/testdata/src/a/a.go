package a

type Store interface { // want `interface Store is defined next to its implementation memStore`
	Get(key string) string
}

type memStore struct{}

func (m *memStore) Get(key string) string { return "" }

type Fetcher interface {
	Fetch(url string) ([]byte, error)
}

type httpFetcher struct{}

func (h *httpFetcher) Fetch(url string) ([]byte, error) { return nil, nil }

func NewClient(f Fetcher) {}
