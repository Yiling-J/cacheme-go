package fetcher

import (
	"context"
	"errors"
	"sync"

	"github.com/Yiling-J/cacheme-go/integration/cacheme/store"
	"github.com/Yiling-J/cacheme-go/integration/model"
)

var (
	Tester                        string
	FixCacheStoreCounter          int
	SimpleCacheStoreCounter       int
	FooCacheStoreCounter          int
	FooPCacheStoreCounter         int
	FooMapCacheStoreCounter       int
	FooListCacheStoreCounter      int
	FooListPCacheStoreCounter     int
	SimpleFlightCacheStoreCounter int
	SimpleMultiCacheStoreCounter  int

	mu sync.Mutex
)

func Setup() {

	store.FixCacheStore.Fetch = func(ctx context.Context) (string, error) {
		mu.Lock()
		FixCacheStoreCounter++
		mu.Unlock()
		return "fix", nil
	}

	store.SimpleCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
		mu.Lock()
		SimpleCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			return "", nil
		}
		if ID == "E" {
			return "", errors.New("")
		}
		return ID + Tester, nil
	}

	store.SimpleMultiCacheStore.Fetch = func(ctx context.Context, Foo, Bar, ID string) (string, error) {
		mu.Lock()
		SimpleMultiCacheStoreCounter++
		mu.Unlock()
		return Foo + Bar + ID, nil
	}

	store.SimpleFlightCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
		mu.Lock()
		SimpleFlightCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			return "", nil
		}
		if ID == "E" {
			return "", errors.New("")
		}
		return ID + Tester, nil
	}

	store.FooCacheStore.Fetch = func(ctx context.Context, ID string) (model.Foo, error) {
		mu.Lock()
		FooCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			return model.Foo{}, nil
		}
		if ID == "E" {
			return model.Foo{}, errors.New("")
		}
		return model.Foo{
			Name: ID + Tester,
			Bar:  model.Bar{Name: ID + Tester + "bar"},
			BarP: &model.Bar{Name: ID + Tester + "bar"},
		}, nil
	}

	store.FooPCacheStore.Fetch = func(ctx context.Context, ID string) (*model.Foo, error) {
		mu.Lock()
		FooPCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			return nil, nil
		}
		if ID == "E" {
			return nil, errors.New("")
		}
		return &model.Foo{
			Name: ID + Tester,
			Bar:  model.Bar{Name: ID + Tester + "bar"},
			BarP: &model.Bar{Name: ID + Tester + "bar"},
		}, nil
	}

	store.FooMapCacheStore.Fetch = func(ctx context.Context, ID string) (map[string]string, error) {
		mu.Lock()
		FooMapCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			return nil, nil
		}
		if ID == "E" {
			return nil, errors.New("")
		}
		return map[string]string{"name": ID + Tester}, nil
	}

	store.FooListCacheStore.Fetch = func(ctx context.Context, ID string) ([]model.Foo, error) {
		mu.Lock()
		FooListCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			var zero []model.Foo
			return zero, nil
		}
		if ID == "E" {
			return []model.Foo{}, errors.New("")
		}
		return []model.Foo{{Name: ID + Tester, Bar: model.Bar{Name: ID + Tester + "bar"},
			BarP: &model.Bar{Name: ID + Tester + "bar"}}}, nil
	}

	store.FooListPCacheStore.Fetch = func(ctx context.Context, ID string) ([]*model.Foo, error) {
		mu.Lock()
		FooListPCacheStoreCounter++
		mu.Unlock()

		if ID == "" {
			var zero []*model.Foo
			return zero, nil
		}
		if ID == "E" {
			return []*model.Foo{}, errors.New("")
		}
		return []*model.Foo{{Name: ID + Tester, Bar: model.Bar{Name: ID + Tester + "bar"},
			BarP: &model.Bar{Name: ID + Tester + "bar"}}}, nil
	}

	store.BarCacheStore.Fetch = func(ctx context.Context, ID string) (model.Bar, error) {
		return model.Bar{}, nil
	}
}
