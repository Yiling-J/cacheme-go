# cacheme - Redis Caching Framework For Go
![example workflow](https://github.com/Yiling-J/cacheme-go/actions/workflows/go.yml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/Yiling-J/cacheme-go?style=flat-square)

[English](README.md) | [中文](README_zh.md)

- **Statically Typed** - 100% statically typed using code generation.
- **Scale Efficiently** - thundering herd protection via pub/sub.
- **Cluster Support** - same API for redis & redis cluster.
- **Memoize** - dynamic key generation based on code generation.
- **Versioning** - cache versioning for better management.
- **Pipeline** - reduce io cost by redis pipeline.

Read this first: [Caches, Promises and Locks](https://redis.com/blog/caches-promises-locks/). This is how caching part works in cacheme.

## Installation
```console
go get github.com/Yiling-J/cacheme-go/cmd
```
After installing `cacheme-go` codegen, go to the root directory of your project, and run:
```console
go run github.com/Yiling-J/cacheme-go/cmd init
```
The command above will generate `cacheme` directory under root directory:
```console {12-20}
└── cacheme
    ├── fetcher
    │   └── fetcher.go
    └── schema
        └── schema.go
```

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

	// extra imports in generated file
	Imports = []string{}

	// store schemas
	Stores = []*cacheme.StoreSchema{
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}
)
```

More details [here](#schema-definition)

## Store Generation
Run code generation from the root directory of the project as follows:
```console
go run github.com/Yiling-J/cacheme-go/cmd generate
```
This produces the following files:
```console {12-20}
└── cacheme
    ├── fetcher
    │   └── fetcher.go
    ├── schema
    │   └── schema.go
    └── store.go
```
`store.go` is generated based on schemas in `schema.go`. Adding more schemas and run `generate` again.

## Add Fetcher
Each cache store should provide a fetch function in `fetcher.go`:
```go
func Setup() {
	cacheme.SimpleCacheStore.Fetch = func(ctx context.Context, ID string) (string, error) {
		return ID, nil
	}
}
```

## Use Your Stores
Create a client and get data, fetch function will be called if cache not exist:
```go
import (
	"your_project/cacheme"
	"your_project/cacheme/fetcher"
)

func example() (string, error) {
	ctx := context.TODO()
	fetcher.Setup()
	client := cacheme.New(
		redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		}),
	)
	store := client.SimpleCacheStore
	result, err := store.Get(ctx, "foo")
	if err != nil {
		return "", err
	}
	return result, err
}
```

Redis Cluster:
```go
import (
	"your_project/cacheme"
	"your_project/cacheme/fetcher"
)

func example() (string, error) {
	ctx := context.TODO()
	fetcher.Setup()
	client := cacheme.NewCluster(
		redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: []string{
				":7000",
				":7001",
				":7002"},
		}),
	)
	store := client.SimpleCacheStore
	result, err := store.Get(ctx, "foo")
	if err != nil {
		return "", err
	}
	return result, err
}
```
Invalid your cache:
```go
err := store.Invalid(ctx, "foo")
```
Update your cache:
```go
err := store.Update(ctx, "foo")
```
Redis pipeline(using same client, skip client creation here):
```go
import cachemego "github.com/Yiling-J/cacheme-go"

...
pipeline := cachemego.NewPipeline(client.Redis())
ids := []string{"1", "2", "3", "4"}
var ps []*cacheme.SimplePromise
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
Mixed pipeline:
```go
import cachemego "github.com/Yiling-J/cacheme-go"

...
// same pipeline for different stores
pipeline := cachemego.NewPipeline(client.Redis())

ids := []string{"1", "2", "3", "4"}
var ps []*cacheme.SimplePromise // cache string
var psf []*cacheme.FooPromise // cache model.Foo struct
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
fmt.Println(err)
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
Invalid all cache with version:
```go
// invalid all version 1 simple cache
client.SimpleCacheStore.InvalidAll(ctx, 1)
```

## Schema Definition
Each schema has 5 fields:
- **Name** - store name, will be struct name in generated code, capital first.
- **Key** - key with variable using **go template syntax**, Variable name will be used in code generation.
- **To** - cached value type string, will be used in code generation. Examples:
	- string: `"string"`
	- struct: `"model.Foo"`
	- struct pointer: `"*model.Foo"`
	- slice: `"[]model.Foo"`
- **Version** - version number, for schema change.
- **TTL** - redis ttl using go time.
- **Singleflight** - bool, if `true`, cocurrency requests to **same key** on **same machine** will call Redis only once 

#### Notes:
- Duplicate name/key is not allowed.

- If set `Singleflight` to `true`, Redis `GET` command will be wrapped in a [**singleflight**](https://pkg.go.dev/golang.org/x/sync/singleflight), so **cocurrency requests of same key** will call Redis only once. This option only works for `store.Get` method, not `store.GetP`. Make it optional because in some situations, for example aws Lambda, `singleflight` is useless.

- Full redis key has 3 parts: **prefix** + **schema key** + **version**.
	Schema Key`category:{{.categoryID}}:book:{{.bookID}}` with prefix `cacheme`, version 1 will generate key:
	```
	cacheme:category:1:book:3:v1
	```
	Also you will see `categoryID` and `bookID` in generated code, as fetch func params.

- In `schema.go`, there is an `imports` part:
	```go
	Imports = []string{}
	```
	If you use structs in `To`, don't forget to add struct path here, so code generation can work:
	```go
	// we have model.Foo struct in schema
	Imports = []string{"your_project/model"}
	```
