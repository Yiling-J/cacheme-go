package integration_test

import (
	"context"
	"testing"

	cachemeo "github.com/Yiling-J/cacheme-go/cacheme"
	"github.com/Yiling-J/cacheme-go/integration/cacheme"
	"github.com/Yiling-J/cacheme-go/integration/cacheme/fetcher"
	"github.com/Yiling-J/cacheme-go/integration/model"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func Cacheme() *cacheme.Client {
	return cacheme.New(redis.NewClient(
		&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
	))
}

func CleanRedis() {
	client := redis.NewClient(
		&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
	)
	client.FlushAll(context.TODO())
}

func RestCounter() {
	fetcher.FooCacheStoreCounter = 0
	fetcher.FooListCacheStoreCounter = 0
	fetcher.FooListPCacheStoreCounter = 0
	fetcher.FooMapCacheStoreCounter = 0
	fetcher.FooPCacheStoreCounter = 0
	fetcher.SimpleCacheStoreCounter = 0
}

func TestCacheType(t *testing.T) {
	tests := []struct {
		name             string
		id               string
		expectedSimple   string
		expectedFooMap   map[string]string
		expectedFoo      model.Foo
		expectedFooP     *model.Foo
		expectedFooList  []model.Foo
		expectedFooListP []*model.Foo
	}{
		{
			name: "zero value",
		},
		{
			name:             "simple",
			id:               "1",
			expectedSimple:   "1",
			expectedFooMap:   map[string]string{"name": "1"},
			expectedFoo:      model.Foo{Name: "1"},
			expectedFooP:     &model.Foo{Name: "1"},
			expectedFooList:  []model.Foo{{Name: "1"}},
			expectedFooListP: []*model.Foo{{Name: "1"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetcher.Setup()
			client := Cacheme()
			defer CleanRedis()
			RestCounter()
			ctx := context.Background()

			stores := []cachemeo.CacheStore{
				client.SimpleCacheStore,
				client.FooMapCacheStore,
				client.FooPCacheStore,
				client.FooCacheStore,
				client.FooListCacheStore,
				client.FooListPCacheStore,
			}

			for _, store := range stores {
				err := store.AddMemoLock()
				require.Nil(t, err)
			}

			// test get without cache
			r1, err := client.SimpleCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedSimple, r1)

			r2, err := client.FooCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFoo, r2)

			r3, err := client.FooPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooP, r3)

			r4, err := client.FooMapCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooMap, r4)

			r5, err := client.FooListCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooList, r5)

			r6, err := client.FooListPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooListP, r6)

			// test get with cache, counter should equal 1
			r1, err = client.SimpleCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedSimple, r1)
			require.Equal(t, 1, fetcher.SimpleCacheStoreCounter)

			r2, err = client.FooCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFoo, r2)
			require.Equal(t, 1, fetcher.FooCacheStoreCounter)

			r3, err = client.FooPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooP, r3)
			require.Equal(t, 1, fetcher.FooPCacheStoreCounter)

			r4, err = client.FooMapCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooMap, r4)
			require.Equal(t, 1, fetcher.FooMapCacheStoreCounter)

			r5, err = client.FooListCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooList, r5)
			require.Equal(t, 1, fetcher.FooListCacheStoreCounter)

			r6, err = client.FooListPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, tc.expectedFooListP, r6)
			require.Equal(t, 1, fetcher.FooListPCacheStoreCounter)

			// test invalid cache
			// test get again
			// test cache warm
		},
		)
	}
}
