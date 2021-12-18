// Code generated by cacheme, DO NOT EDIT.
package store

import (
	cacheme "github.com/Yiling-J/cacheme-go"

	"github.com/Yiling-J/cacheme-go/integration/cacheme/schema"

	"strconv"
)

const (
	Hit   = "HIT"
	Miss  = "MISS"
	Fetch = "FETCH"
)

// Client is the cacheme client for all stores.
type Client struct {
	FixCacheStore *FixCache

	SimpleCacheStore *SimpleCache

	SimpleMultiCacheStore *SimpleMultiCache

	FooMapCacheStore *FooMapCache

	FooCacheStore *FooCache

	BarCacheStore *BarCache

	FooPCacheStore *FooPCache

	FooListCacheStore *FooListCache

	FooListPCacheStore *FooListPCache

	FooMapSCacheStore *FooMapSCache

	SimpleFlightCacheStore *SimpleFlightCache

	redis   cacheme.RedisClient
	cluster bool
	logger  cacheme.Logger
}

// Redis return the current redis client.
func (c *Client) Redis() cacheme.RedisClient {
	return c.redis
}

// SetLogger set logger interface for current client.
func (c *Client) SetLogger(l cacheme.Logger) {
	c.logger = l
}

// NewPipeline returns a new cacheme pipeline.
func (c *Client) NewPipeline() *cacheme.CachePipeline {
	return cacheme.NewPipeline(c.redis)
}

var FixCacheStore = &FixCache{tag: "Fix", singleflight: false, metadata: true}

var SimpleCacheStore = &SimpleCache{tag: "Simple", singleflight: false, metadata: true}

var SimpleMultiCacheStore = &SimpleMultiCache{tag: "SimpleMulti", singleflight: false, metadata: true}

var FooMapCacheStore = &FooMapCache{tag: "FooMap", singleflight: false, metadata: true}

var FooCacheStore = &FooCache{tag: "Foo", singleflight: false, metadata: true}

var BarCacheStore = &BarCache{tag: "Bar", singleflight: false, metadata: false}

var FooPCacheStore = &FooPCache{tag: "FooP", singleflight: false, metadata: true}

var FooListCacheStore = &FooListCache{tag: "FooList", singleflight: false, metadata: true}

var FooListPCacheStore = &FooListPCache{tag: "FooListP", singleflight: false, metadata: true}

var FooMapSCacheStore = &FooMapSCache{tag: "FooMapS", singleflight: false, metadata: true}

var SimpleFlightCacheStore = &SimpleFlightCache{tag: "SimpleFlight", singleflight: true, metadata: true}

func init() {

	FixCacheStore.versionString = strconv.Itoa(schema.Stores[0].Version.(int))

	SimpleCacheStore.versionFunc = schema.Stores[1].Version.(func() string)

	SimpleMultiCacheStore.versionString = strconv.Itoa(schema.Stores[2].Version.(int))

	FooMapCacheStore.versionString = schema.Stores[3].Version.(string)

	FooCacheStore.versionString = schema.Stores[4].Version.(string)

	BarCacheStore.versionFunc = schema.Stores[5].Version.(func() string)

	FooPCacheStore.versionString = strconv.Itoa(schema.Stores[6].Version.(int))

	FooListCacheStore.versionString = strconv.Itoa(schema.Stores[7].Version.(int))

	FooListPCacheStore.versionString = strconv.Itoa(schema.Stores[8].Version.(int))

	FooMapSCacheStore.versionString = strconv.Itoa(schema.Stores[9].Version.(int))

	SimpleFlightCacheStore.versionString = strconv.Itoa(schema.Stores[10].Version.(int))

}

// New create a new cacheme client with given redis client.
func New(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis}

	client.FixCacheStore = FixCacheStore.clone(client.redis)
	client.FixCacheStore.setClient(client)

	client.SimpleCacheStore = SimpleCacheStore.clone(client.redis)
	client.SimpleCacheStore.setClient(client)

	client.SimpleMultiCacheStore = SimpleMultiCacheStore.clone(client.redis)
	client.SimpleMultiCacheStore.setClient(client)

	client.FooMapCacheStore = FooMapCacheStore.clone(client.redis)
	client.FooMapCacheStore.setClient(client)

	client.FooCacheStore = FooCacheStore.clone(client.redis)
	client.FooCacheStore.setClient(client)

	client.BarCacheStore = BarCacheStore.clone(client.redis)
	client.BarCacheStore.setClient(client)

	client.FooPCacheStore = FooPCacheStore.clone(client.redis)
	client.FooPCacheStore.setClient(client)

	client.FooListCacheStore = FooListCacheStore.clone(client.redis)
	client.FooListCacheStore.setClient(client)

	client.FooListPCacheStore = FooListPCacheStore.clone(client.redis)
	client.FooListPCacheStore.setClient(client)

	client.FooMapSCacheStore = FooMapSCacheStore.clone(client.redis)
	client.FooMapSCacheStore.setClient(client)

	client.SimpleFlightCacheStore = SimpleFlightCacheStore.clone(client.redis)
	client.SimpleFlightCacheStore.setClient(client)

	client.logger = &cacheme.NOPLogger{}
	return client
}

// NewCluster create a new cacheme cluster client with given redis client.
func NewCluster(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis, cluster: true}

	client.FixCacheStore = FixCacheStore.clone(client.redis)
	client.FixCacheStore.setClient(client)

	client.SimpleCacheStore = SimpleCacheStore.clone(client.redis)
	client.SimpleCacheStore.setClient(client)

	client.SimpleMultiCacheStore = SimpleMultiCacheStore.clone(client.redis)
	client.SimpleMultiCacheStore.setClient(client)

	client.FooMapCacheStore = FooMapCacheStore.clone(client.redis)
	client.FooMapCacheStore.setClient(client)

	client.FooCacheStore = FooCacheStore.clone(client.redis)
	client.FooCacheStore.setClient(client)

	client.BarCacheStore = BarCacheStore.clone(client.redis)
	client.BarCacheStore.setClient(client)

	client.FooPCacheStore = FooPCacheStore.clone(client.redis)
	client.FooPCacheStore.setClient(client)

	client.FooListCacheStore = FooListCacheStore.clone(client.redis)
	client.FooListCacheStore.setClient(client)

	client.FooListPCacheStore = FooListPCacheStore.clone(client.redis)
	client.FooListPCacheStore.setClient(client)

	client.FooMapSCacheStore = FooMapSCacheStore.clone(client.redis)
	client.FooMapSCacheStore.setClient(client)

	client.SimpleFlightCacheStore = SimpleFlightCacheStore.clone(client.redis)
	client.SimpleFlightCacheStore.setClient(client)

	client.logger = &cacheme.NOPLogger{}
	return client
}