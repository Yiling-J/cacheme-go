# cacheme - Redis Caching Framework For Go
![example workflow](https://github.com/Yiling-J/cacheme-go/actions/workflows/go.yml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/Yiling-J/cacheme-go?style=flat-square)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge-flat.svg)](https://github.com/avelino/awesome-go#orm)

[English](README.md) | [δΈ­ζ](README_zh.md)

- **Statically Typed** - 100% statically typed using code generation. Drop-in replacement, no reflect/type-assertion.
- **Scale Efficiently** - thundering herd protection via pub/sub.
- **Cluster Support** - same API for redis & redis cluster.
- **Memoize** - dynamic key params based on code generation.
- **Versioning** - cache versioning for better management.
- **Pipeline** - reduce io cost by redis pipeline.

π Read this first: [Caches, Promises and Locks](https://redis.com/blog/caches-promises-locks/). This is how caching part works in cacheme.

π Real world example with [Echo](https://github.com/labstack/echo) and [Ent](https://github.com/ent/ent): https://github.com/Yiling-J/echo-ent-cacheme-example
```go
// old
id, err := strconv.ParseInt(c.Param("id"), 10, 64)
comment, err := ent.Comment.Get(context.Background(), int(id))

// new
comment, err := cacheme.CommentCacheStore.Get(c.Request().Context(), c.Param("id"))
```

## Installation
```console
go get github.com/Yiling-J/cacheme-go/cmd
```
After installing `cacheme-go` codegen, go to the root directory(or the directory you think cacheme should stay) of your project, and run:
```console
go run github.com/Yiling-J/cacheme-go/cmd init
```
The command above will generate `cacheme` directory under current directory:
```console {12-20}
βββ cacheme
    βββ fetcher
 Β Β  βΒ Β  βββ fetcher.go
    βββ schema
        βββ schema.go
```
It's up to you where the cacheme directory should be, just remember to use the right directory in **Store Generation** step.

## Add Schema
Edit `schema.go` and add some schemas:
```go
package schema

import (
	"time"
	cacheme "github.com/Yiling-J/cacheme-go"
)

var (
	// default prefix for redis keys
	Prefix = "cacheme"
	// store schemas
	Stores = []*cacheme.StoreSchema{
		{
			Name:         "Simple",
			Key:          "simple:{{.ID}}",
			To:           "",
			Version:      1,
			TTL:          5 * time.Minute,
			Singleflight: false,
			MetaData: false,
		},
	}
)
```

More details [here](#schema-definition)

## Store Generation
Run code generation from the root directory of the project as follows:
```console
# this will use default schema path ./cacheme/schema
go run github.com/Yiling-J/cacheme-go/cmd generate
```
Or you can use custom schema path:
```console
go run github.com/Yiling-J/cacheme-go/cmd generate ./yours/cacheme/schema
```

This produces the following files:
```console {12-20}
βββ cacheme
 Β Β  βββ fetcher
 Β Β  βΒ Β  βββ fetcher.go
    βββ schema
    βΒ Β  βββ schema.go
    βββ store
    β   βββ base.go
    β   βββ simple.go
    βββ store.go
```
If you update schema, just run `generate` again.

## Add Fetcher
Each cache store can provide a fetch function in `fetcher.go`, you should call this `Setup` function before create client:
```go
import "your/cacheme/store"

func Setup() {
	store.SimpleCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
		return ID, nil
	}
}
```
**You can setup fetch functions in any place, using any pattern, not restricted to this file. Just make sure you have a fetch function when using store.**

## Use Your Stores

### Create client and setup fetcher
```go
import (
	"your_project/cacheme"
	"your_project/cacheme/fetcher"
)

func main() {
	// setup fetcher
	fetcher.Setup()
	// create client
	client := cacheme.New(
		redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		}),
	)
	// or cluster client
	client := cacheme.NewCluster(
		redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: []string{
				":7000",
				":7001",
				":7002"},
		}),
	)
}
```
### Store API
#### Get single result: `Get`
Get cached result. If not in cache, call fetch function and store data to Redis.
```go
// "foo" is the {{.ID}} part of the schema
result, err := client.SimpleCacheStore.Get(ctx, "foo")
```
#### Get pipeline results: `GetP`
Get multiple keys from multiple stores using pipeline.
For each key, if not in cache, call fetch function and store data to Redis.
- single store
```go
pipeline := client.NewPipeline()
ids := []string{"1", "2", "3", "4"}
var ps []*store.SimplePromise
for _, i := range ids {
	promise, err := client.SimpleCacheStore.GetP(ctx, pipeline, i)
	ps = append(ps, promise)
}
err = pipeline.Execute(ctx)
fmt.Println(err)

for _, promise := range ps {
	r, err := promise.Result()
	fmt.Println(r, err)
}
```
Consider using `GetM` API for single store, see `GetM` example below.

- multiple stores
```go
// same pipeline for different stores
pipeline := client.NewPipeline()

ids := []string{"1", "2", "3", "4"}
var ps []*store.SimplePromise // cache string
var psf []*store.FooPromise // cache model.Foo struct
for _, i := range ids {
	promise, err := client.SimpleCacheStore.GetP(ctx, pipeline, i)
	ps = append(ps, promise)
}
for _, i := range ids {
	promise, err := client.FooCacheStore.GetP(ctx, pipeline, i)
	psf = append(psf, promise)
}
// execute only once
err = pipeline.Execute(ctx)
// simple store results
for _, promise := range ps {
	r, err := promise.Result()
	fmt.Println(r, err)
}
// foo store results
for _, promise := range psf {
	r, err := promise.Result()
	fmt.Println(r, err)
}

```
#### Get multiple results from single store: `GetM`
Get multiple keys from same store, also using Redis pipeline.
For each key, if not in cache, call fetch function and store data to Redis.
```go
qs, err := client.SimpleCacheStore.GetM("foo").GetM("bar").GetM("xyz").Do(ctx)
// qs is a queryset struct, support two methods: GetSlice and Get
// GetSlice return ordered results slice
r, err := qs.GetSlice() // r: {foo_result, bar_result, xyz_result}
// Get return result of given param
r, err := qs.Get("foo") // r: foo_result
r, err := qs.Get("bar") // r: bar_result
r, err := qs.Get("fake") // error, because "fake" not in queryset
```
You can also initialize a getter using `MGetter`
```go
getter := client.SimpleCacheStore.MGetter()
for _, id := range ids {
	getter.GetM(id)
}
qs, err := getter.Do(c.Request().Context())
```
#### Invalid single cache: `Invalid`
```go
err := client.SimpleCacheStore.Invalid(ctx, "foo")
```
#### Update single cache: `Update`
```go
err := client.SimpleCacheStore.Update(ctx, "foo")
```
#### Invalid all keys: `InvalidAll`
Only works when you enable `MetaData` option in schema.
```go
// invalid all version 1 simple cache
client.SimpleCacheStore.InvalidAll(ctx, "1")
```

## Schema Definition
Each schema has 5 fields:
- **Name** - store name, will be struct name in generated code, capital first.
- **Key** - key with variable using **go template syntax**, Variable name will be used in code generation.
- **To** - cached value, type of value will be used in code generation. Examples:
	- string: `""`
	- int: `1`
	- struct: `model.Foo{}`
	- struct pointer: `&model.Foo{}`
	- slice: `[]model.Foo{}`
	- map: `map[model.Foo]model.Bar{}`
- **Version** - version interface, can be `string`, `int`, or callable `func() string`.
- **TTL** - redis ttl using go time.
- **Singleflight** - bool, if `true`, concurrent requests to **same key** on **same executable** will call Redis only once
- **MetaData** - bool, if `true`, each store will save all generated keys to a Redis Set, so `InvalidAll` method can work.

#### Notes:
- Duplicate name/key is not allowed.
- Everytime you update schema, run code generation again.
- Not all store API support `Singleflight` option:
	- `Get`: support.
	- `GetM`: support. singleflight key will be the combination of all keys, order by alphabetical.
	```go
	// these two will use same singleflight group key
	store.GetM("foo").GetM("bar").GetM("xyz").Do(ctx)
	Store.GetM("bar").GetM("foo").GetM("xyz").Do(ctx)
	```
	- `GetP`: not support.
- `Version` callable can help you managing version better. Example:
	```go
	// models.go
	const FooCacheVersion = "1"
	type Foo struct {}
	const BarCacheVersion = "1"
	type Bar struct {Foo: Foo}
	```
	```go
	// schema.go
	// version has 3 parts: foo version & bar version & global version number
	// if you change struct, update FooCacheVersion or BarCacheVersion
	// if you change fetcher function or ttl or something else, change global version number
	{
		Name:    "Bar",
		Key:     "bar:{{.ID}}:info",
		To:      model.Bar{},
		Version: func() string {return model.FooCacheVersion + model.BarCacheVersion + "1"},
		TTL:     5 * time.Minute,
	},
	```
- If set `Singleflight` to `true`, Cacheme `Get` command will be wrapped in a [**singleflight**](https://pkg.go.dev/golang.org/x/sync/singleflight), so **concurrent requests to same key** will call `Redis` only once. Let's use some example to explain this:
	- you have some products to sell, and thousands people will view the detail at same time, so the product key `product:1:info` may be hit 100000 times per second. Now you should turn on **singleflight**, and the actually redis hit may reduce to 5000.
	- you have cache for user shopping cart `user:123:cart`, only the user himself can see that. Now no need to use **singleflight**, becauese there shouldn't be concurrent requests to that key.
	- you are using serverless platform, AWS Lambda or similar. So each request runs in isolated environment, can't talk to each other through channels. Then **singleflight** make no sense.
- Full redis key has 3 parts: **prefix** + **schema key** + **version**.
	Schema Key`category:{{.categoryID}}:book:{{.bookID}}` with prefix `cacheme`, version 1 will generate key:
	```
	cacheme:category:1:book:3:v1
	```
	Also you will see `categoryID` and `bookID` in generated code, as fetch func params.

## Logger
You can use custom logger with cacheme, your logger should implement cacheme logger interface:
```go
type Logger interface {
	Log(store string, key string, op string)
}
```
Here `store` is the store tag, `key` is cache key without prefix, `op` is operation type.
Default logger is `NOPLogger`, just return and do nothing.

#### Set client logger:
```go
logger := &YourCustomLogger{}
client.SetLogger(logger)
```
#### Operation Types:
 - **HIT**: cache hit to redis, if you enable singleflight, grouped requests only log once.
 - **MISS**: cache miss
 - **FETCH**: fetch data from fetcher

## Performance
Parallel benchmarks of Cacheme
 - params: 10000/1000000 hits, 10 keys loop, TTL 10s, `SetParallelism(100)`, singleflight on
```
cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
BenchmarkCachemeGetParallel-12    	   10000	    198082 ns/op
BenchmarkCachemeGetParallel-12    	 1000000	      9501 ns/op
```
