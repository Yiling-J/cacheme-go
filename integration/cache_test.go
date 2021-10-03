package integration_test

import (
	"context"
	"fmt"
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

func CachemeCluster() *cacheme.Client {
	client := redis.NewClusterClient(
		&redis.ClusterOptions{
			Addrs: []string{
				":7000",
				":7001",
				":7002",
				":7003",
				":7004",
				":7005",
			},
		},
	)
	r := client.Ping(context.TODO())
	fmt.Println(r.Result())
	client.ReloadState(context.TODO())

	return cacheme.NewCluster(client)
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

func CleanRedisCluster() {

	client := redis.NewClusterClient(
		&redis.ClusterOptions{
			Addrs: []string{
				":7000",
				":7001",
				":7002",
				":7003",
				":7004",
				":7005"},
		},
	)
	r := client.Ping(context.TODO())
	fmt.Println(r.Result())
	client.ReloadState(context.TODO())

	err := client.ForEachMaster(context.TODO(), func(ctx context.Context, master *redis.Client) error {
		return master.FlushDB(ctx).Err()
	})
	fmt.Println("ERR", err)

}

func RestCounter() {
	fetcher.FooCacheStoreCounter = 0
	fetcher.FooListCacheStoreCounter = 0
	fetcher.FooListPCacheStoreCounter = 0
	fetcher.FooMapCacheStoreCounter = 0
	fetcher.FooPCacheStoreCounter = 0
	fetcher.SimpleCacheStoreCounter = 0
}

func CacheTypeTest(t *testing.T, client *cacheme.Client, cleanFunc func()) {
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
			name:           "simple",
			id:             "1",
			expectedSimple: "1",
			expectedFooMap: map[string]string{"name": "1"},
			expectedFoo: model.Foo{
				Name: "1",
				Bar:  model.Bar{Name: "1bar"},
				BarP: &model.Bar{Name: "1bar"},
			},
			expectedFooP: &model.Foo{
				Name: "1",
				Bar:  model.Bar{Name: "1bar"},
				BarP: &model.Bar{Name: "1bar"},
			},
			expectedFooList: []model.Foo{{
				Name: "1",
				Bar:  model.Bar{Name: "1bar"},
				BarP: &model.Bar{Name: "1bar"},
			}},
			expectedFooListP: []*model.Foo{{
				Name: "1",
				Bar:  model.Bar{Name: "1bar"},
				BarP: &model.Bar{Name: "1bar"},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer cleanFunc()
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

			// test get with cache, counter should be 1
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
			err = client.SimpleCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			err = client.FooCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			err = client.FooMapCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			err = client.FooPCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			err = client.FooListCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			err = client.FooListPCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)

			// test get again,  counter should be 2 now
			_, err = client.SimpleCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.SimpleCacheStoreCounter)

			_, err = client.FooCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.FooCacheStoreCounter)

			_, err = client.FooPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.FooPCacheStoreCounter)

			_, err = client.FooMapCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.FooMapCacheStoreCounter)

			_, err = client.FooListCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.FooListCacheStoreCounter)

			_, err = client.FooListPCacheStore.Get(ctx, tc.id)
			require.Nil(t, err)
			require.Equal(t, 2, fetcher.FooListPCacheStoreCounter)

			// test cache warm
			err = client.SimpleCacheStore.InvalidAll(ctx, 1)
			require.Nil(t, err)
			fetcher.Tester = "test"
			err = client.SimpleCacheStore.Update(ctx, "1")
			require.Nil(t, err)
			r1, err = client.SimpleCacheStore.Get(ctx, "1")
			require.Nil(t, err)
			require.Equal(t, "1test", r1)
			fetcher.Tester = ""
			r1, err = client.SimpleCacheStore.Get(ctx, "1")
			require.Nil(t, err)
			require.Equal(t, "1test", r1)

			err = client.SimpleCacheStore.Update(ctx, "1")
			require.Nil(t, err)
			r1, err = client.SimpleCacheStore.Get(ctx, "1")
			require.Nil(t, err)
			require.Equal(t, "1", r1)

			// test pipeline
			fetcher.SimpleCacheStoreCounter = 0
			pipeline := cachemeo.NewPipeline(client.Redis())
			ids := []string{"1", "2", "3", "4"}
			var ps []*cacheme.SimplePromise
			for _, i := range ids {
				promise, err := client.SimpleCacheStore.GetP(ctx, pipeline, i)
				require.Nil(t, err)
				ps = append(ps, promise)
			}
			err = pipeline.Execute(ctx)
			require.Nil(t, err)

			var results []string
			for _, promise := range ps {
				r, err := promise.Result()
				require.Nil(t, err)
				results = append(results, r)
			}

			require.Equal(t, []string{"1", "2", "3", "4"}, results)
			require.Equal(t, 3, fetcher.SimpleCacheStoreCounter)

			// test mixed pipeline
			fetcher.SimpleCacheStoreCounter = 0
			fetcher.FooCacheStoreCounter = 0
			pipeline = cachemeo.NewPipeline(client.Redis())
			ids = []string{"5", "6", "7", "8"}
			var pss []*cacheme.SimplePromise
			var psf []*cacheme.FooPromise

			for _, i := range ids {
				if i == "5" || i == "7" {
					promise, err := client.SimpleCacheStore.GetP(ctx, pipeline, i)
					require.Nil(t, err)
					pss = append(pss, promise)
					continue
				}

				promisef, err := client.FooCacheStore.GetP(ctx, pipeline, i)
				require.Nil(t, err)
				psf = append(psf, promisef)
			}
			err = pipeline.Execute(ctx)
			require.Nil(t, err)

			var resultSimple []string
			for _, promise := range pss {
				r, err := promise.Result()
				require.Nil(t, err)
				resultSimple = append(resultSimple, r)
			}
			var resultFoo []model.Foo
			for _, promise := range psf {
				r, err := promise.Result()
				require.Nil(t, err)
				resultFoo = append(resultFoo, r)
			}

			require.Equal(t, []string{"5", "7"}, resultSimple)
			require.Equal(t, 2, fetcher.SimpleCacheStoreCounter)

			require.Equal(t, []model.Foo{
				{
					Name: "6",
					Bar:  model.Bar{Name: "6bar"},
					BarP: &model.Bar{Name: "6bar"},
				},
				{
					Name: "8",
					Bar:  model.Bar{Name: "8bar"},
					BarP: &model.Bar{Name: "8bar"},
				},
			}, resultFoo)
			require.Equal(t, 2, fetcher.FooCacheStoreCounter)
		},
		)
	}
}

func TestSingle(t *testing.T) {
	fetcher.Setup()
	client := Cacheme()
	CacheTypeTest(t, client, CleanRedis)
}

func TestCluster(t *testing.T) {
	fetcher.Setup()
	client := CachemeCluster()
	CacheTypeTest(t, client, CleanRedisCluster)
}
