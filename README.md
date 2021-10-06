# cacheme - Redis Caching Framework For Go
![example workflow](https://github.com/Yiling-J/cacheme-go/actions/workflows/go.yml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/Yiling-J/cacheme-go?style=flat-square)

[English](README.md) | [中文](README_zh.md) 

- **Statically Typed** - 100% statically typed using code generation.
- **Scale Efficiently** - thundering herd protection via pub/sub.
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
`store.go` is generated based on schemas in `schema.go`.

## Add Fetcher
