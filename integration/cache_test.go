package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Yiling-J/cacheme-go/integration/cacheme"
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

func TestCachemeBasic(t *testing.T) {
	client := Cacheme()
	defer CleanRedis()
	store := client.FooBarCacheStore

	err := store.AddMemoLock()
	require.Nil(t, err)

	call := 0
	var mu sync.Mutex
	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		time.Sleep(1 * time.Second)
		mu.Lock()
		call++
		mu.Unlock()
		return fooID + barID, nil
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	// should finish in seconds
	for i := 1; i <= 2000; i++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			if c%2 == 1 {
				r, err := store.Get(ctx, "i", "o")
				require.Nil(t, err)
				require.Equal(t, "io", r)
			} else {
				r, err := store.Get(ctx, "i", "a")
				require.Nil(t, err)
				require.Equal(t, "ia", r)
			}
		}(i)
	}

	wg.Wait()

	// fetch function called only twice
	require.Equal(t, 2, call)
}

func TestCacheStruct(t *testing.T) {
	ctx := context.TODO()
	client := Cacheme()
	defer CleanRedis()
	oneStore := client.FooOneCacheStore
	listStore := client.FooListCacheStore

	err := oneStore.AddMemoLock()
	require.Nil(t, err)
	err = listStore.AddMemoLock()
	require.Nil(t, err)

	counter1 := 0
	counter2 := 0

	oneStore.Fetch = func(ctx context.Context, fooID string) (*model.Foo, error) {
		counter1++
		return &model.Foo{ID: 1, Name: fooID}, nil
	}

	listStore.Fetch = func(ctx context.Context) ([]*model.Foo, error) {
		counter2++
		return []*model.Foo{{ID: 1, Name: "foo"}}, nil
	}

	_, err = oneStore.Get(ctx, "a")
	require.Nil(t, err)
	r, err := oneStore.Get(ctx, "a")
	require.Nil(t, err)
	require.Equal(t, r, &model.Foo{ID: 1, Name: "a"})

	_, err = listStore.Get(ctx)
	require.Nil(t, err)
	r2, err := listStore.Get(ctx)
	require.Nil(t, err)
	require.Equal(t, r2, []*model.Foo{{ID: 1, Name: "foo"}})

	require.Equal(t, 1, counter1)
	require.Equal(t, 1, counter2)

}

func TestCachemeGroupInvalid(t *testing.T) {
	ctx := context.TODO()
	client := Cacheme()
	defer CleanRedis()

	store := client.FooBarCacheStore
	err := store.AddMemoLock()
	require.Nil(t, err)
	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		return "foo", nil
	}

	keys := []string{"a", "b", "c", "d"}

	for _, key := range keys {
		r, err := store.Get(ctx, "i", key)
		require.Nil(t, err)
		require.Equal(t, "foo", r)
	}

	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		return "bar", nil
	}

	for _, key := range keys {
		r, err := store.Get(ctx, "i", key)
		require.Nil(t, err)
		require.Equal(t, "foo", r)
	}

	err = store.InvalidAll(ctx, 1)
	require.Nil(t, err)

	time.Sleep(30 * time.Millisecond)

	for _, key := range keys {
		r, err := store.Get(ctx, "i", key)
		require.Nil(t, err)
		require.Equal(t, "bar", r)
	}

}

func TestCachemeUpdate(t *testing.T) {
	ctx := context.TODO()
	client := Cacheme()
	defer CleanRedis()

	store := client.FooBarCacheStore
	err := store.AddMemoLock()
	require.Nil(t, err)
	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		return "foo", nil
	}

	r, err := store.Get(ctx, "i", "a")
	require.Nil(t, err)
	require.Equal(t, "foo", r)

	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		return "bar", nil
	}

	r, err = store.Get(ctx, "i", "a")
	require.Nil(t, err)
	require.Equal(t, "foo", r)

	err = store.Update(ctx, "i", "a")
	require.Nil(t, err)

	r, err = store.Get(ctx, "i", "a")
	require.Nil(t, err)
	require.Equal(t, "bar", r)

}

func TestPipeline(t *testing.T) {
	ctx := context.TODO()
	client := Cacheme()
	defer CleanRedis()

	store := client.FooBarCacheStore
	err := store.AddMemoLock()
	require.Nil(t, err)

	var mu sync.Mutex
	var results []string
	store.Fetch = func(ctx context.Context, fooID, barID string) (string, error) {
		mu.Lock()
		results = append(results, fooID)
		mu.Unlock()
		return fooID + barID, nil
	}

	keys := []string{"a", "b", "c", "d"}
	pipeline := client.NewPipeline()
	var promise []*cacheme.FooBarPromise
	for _, k := range keys {
		p, err := store.GetP(ctx, pipeline, k, "bar")
		require.Nil(t, err)
		promise = append(promise, p)
	}
	err = pipeline.Execute(ctx)
	require.Nil(t, err)

	for i, p := range promise {
		r, err := p.Result()
		require.Nil(t, err)
		require.Equal(t, keys[i]+"bar", r)
	}
	require.ElementsMatch(t, []string{"a", "b", "c", "d"}, results)

	// do again, get cached results
	results = []string{}
	promise = []*cacheme.FooBarPromise{}
	pipeline = client.NewPipeline()
	for _, k := range keys {
		p, err := store.GetP(ctx, pipeline, k, "bar")
		require.Nil(t, err)
		promise = append(promise, p)
	}
	err = pipeline.Execute(ctx)
	require.Nil(t, err)

	for i, p := range promise {
		r, err := p.Result()
		require.Nil(t, err)
		require.Equal(t, keys[i]+"bar", r)
	}
	require.Equal(t, []string{}, results)

}
