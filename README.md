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
import "your_project/cacheme"

func example() (string, error) {
	ctx := context.TODO()
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
