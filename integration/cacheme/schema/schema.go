package schema

import (
	"strconv"
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
	"github.com/Yiling-J/cacheme-go/integration/model"
)

var (
	// default prefix for redis keys
	Prefix = "cacheme"

	// store templates
	Stores = []*cacheme.StoreSchema{
		{
			Name:    "Fix",
			Key:     "fix",
			To:      "",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "",
			Version: func() string { return "1" },
			TTL:     5 * time.Minute,
		},
		{
			Name:    "SimpleMulti",
			Key:     "simplem:{{.Foo}}:{{.Bar}}:{{.ID}}",
			To:      "",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooMap",
			Key:     "foomap:{{.ID}}",
			To:      map[string]string{},
			Version: "1",
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Foo",
			Key:     "foo:{{.ID}}:info",
			To:      model.Foo{},
			Version: "1",
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Bar",
			Key:     "bar:{{.ID}}:info",
			To:      model.Bar{},
			Version: func() string { return strconv.Itoa(model.BarVersion) },
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooP",
			Key:     "foop:{{.ID}}:info",
			To:      &model.Foo{},
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooList",
			Key:     "foo:list:{{.ID}}",
			To:      []model.Foo{},
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooListP",
			Key:     "foo:listp:{{.ID}}",
			To:      []*model.Foo{},
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "FooMapS",
			Key:     "foo:maps:{{.ID}}",
			To:      map[model.Foo]model.Bar{},
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:         "SimpleFlight",
			Key:          "simple:flight:{{.ID}}",
			To:           "",
			Version:      1,
			TTL:          5 * time.Minute,
			Singleflight: true,
		},
	}
)
