package fetcher

import (
	"context"
	"errors"
	"sync"

	"github.com/Yiling-J/cacheme-go/integration/cacheme"
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

	mu sync.Mutex
)

func Setup() {

	cacheme.FixCacheStore.Fetch = func(ctx context.Context) (string, error) {
		mu.Lock()
		FixCacheStoreCounter++
		mu.Unlock()
		return "fix", nil
	}

	cacheme.SimpleCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
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

	cacheme.SimpleMultiCacheStore.Fetch = func(ctx context.Context, Foo, Bar, ID string) (string, error) {
		return Foo + Bar + ID, nil
	}

	cacheme.SimpleFlightCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
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

	cacheme.FooCacheStore.Fetch = func(ctx context.Context, ID string) (model.Foo, error) {
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

	cacheme.FooPCacheStore.Fetch = func(ctx context.Context, ID string) (*model.Foo, error) {
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

	cacheme.FooMapCacheStore.Fetch = func(ctx context.Context, ID string) (map[string]string, error) {
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

	cacheme.FooListCacheStore.Fetch = func(ctx context.Context, ID string) ([]model.Foo, error) {
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

	cacheme.FooListPCacheStore.Fetch = func(ctx context.Context, ID string) ([]*model.Foo, error) {
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

	cacheme.BarCacheStore.Fetch = func(ctx context.Context, ID string) (model.Bar, error) {
		return model.Bar{}, nil
	}
}
