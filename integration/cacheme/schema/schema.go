package schema

import (
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
)

var (
	// default prefix for redis keys
	Prefix = "cacheme"

	// extra imports in generated file
	Imports = []string{"github.com/Yiling-J/cacheme-go/integration/model"}

	// store templates
	Stores = []*cacheme.StoreSchema{
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooMap",
			Key:     "foomap:{{.ID}}",
			To:      "map[string]string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Foo",
			Key:     "foo:{{.ID}}:info",
			To:      "model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooP",
			Key:     "foop:{{.ID}}:info",
			To:      "*model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooList",
			Key:     "foo:list:{{.ID}}",
			To:      "[]model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooListP",
			Key:     "foo:listp:{{.ID}}",
			To:      "[]*model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}
)
