package schema

import (
	"time"

	"github.com/Yiling-J/cacheme-go/cacheme"
)

var (
	// default prefix for redis keys
	Prefix = "cacheme"

	// extra imports in generated file
	Imports = []string{"github.com/Yiling-J/cacheme-go/integration/model"}

	// store templates
	Stores = []*cacheme.StoreTemplate{
		{
			Name:    "FooBar",
			Key:     "foo:{{.fooID}}:bar:{{.barID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooOne",
			Key:     "foo:{{.fooID}}:info",
			To:      "*model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooList",
			Key:     "foo:list",
			To:      "[]*model.Foo",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}
)
