//nolint
package cacheme

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"text/template"
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
	"github.com/go-redis/redis/v8"

	"github.com/Yiling-J/cacheme-go/integration/cacheme/schema"

	"github.com/Yiling-J/cacheme-go/integration/model"
)

const (
	Hit   = "HIT"
	Miss  = "MISS"
	Fetch = "FETCH"
)

func init() {

	FixCacheStore.version = strconv.Itoa(schema.Stores[0].Version.(int))

	SimpleCacheStore.versionFunc = schema.Stores[1].Version.(func() string)

	SimpleMultiCacheStore.version = strconv.Itoa(schema.Stores[2].Version.(int))

	FooMapCacheStore.version = schema.Stores[3].Version.(string)

	FooCacheStore.version = schema.Stores[4].Version.(string)

	BarCacheStore.versionFunc = schema.Stores[5].Version.(func() string)

	FooPCacheStore.version = strconv.Itoa(schema.Stores[6].Version.(int))

	FooListCacheStore.version = strconv.Itoa(schema.Stores[7].Version.(int))

	FooListPCacheStore.version = strconv.Itoa(schema.Stores[8].Version.(int))

	FooMapSCacheStore.version = strconv.Itoa(schema.Stores[9].Version.(int))

	SimpleFlightCacheStore.version = strconv.Itoa(schema.Stores[10].Version.(int))

}

type Client struct {
	FixCacheStore *fixCache

	SimpleCacheStore *simpleCache

	SimpleMultiCacheStore *simpleMultiCache

	FooMapCacheStore *fooMapCache

	FooCacheStore *fooCache

	BarCacheStore *barCache

	FooPCacheStore *fooPCache

	FooListCacheStore *fooListCache

	FooListPCacheStore *fooListPCache

	FooMapSCacheStore *fooMapSCache

	SimpleFlightCacheStore *simpleFlightCache

	redis   cacheme.RedisClient
	cluster bool
	logger  cacheme.Logger
}

func (c *Client) Redis() cacheme.RedisClient {
	return c.redis
}

func (c *Client) SetLogger(l cacheme.Logger) {
	c.logger = l
}

func New(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis}

	client.FixCacheStore = FixCacheStore.Clone(client.redis)
	client.FixCacheStore.SetClient(client)

	client.SimpleCacheStore = SimpleCacheStore.Clone(client.redis)
	client.SimpleCacheStore.SetClient(client)

	client.SimpleMultiCacheStore = SimpleMultiCacheStore.Clone(client.redis)
	client.SimpleMultiCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone(client.redis)
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone(client.redis)
	client.FooCacheStore.SetClient(client)

	client.BarCacheStore = BarCacheStore.Clone(client.redis)
	client.BarCacheStore.SetClient(client)

	client.FooPCacheStore = FooPCacheStore.Clone(client.redis)
	client.FooPCacheStore.SetClient(client)

	client.FooListCacheStore = FooListCacheStore.Clone(client.redis)
	client.FooListCacheStore.SetClient(client)

	client.FooListPCacheStore = FooListPCacheStore.Clone(client.redis)
	client.FooListPCacheStore.SetClient(client)

	client.FooMapSCacheStore = FooMapSCacheStore.Clone(client.redis)
	client.FooMapSCacheStore.SetClient(client)

	client.SimpleFlightCacheStore = SimpleFlightCacheStore.Clone(client.redis)
	client.SimpleFlightCacheStore.SetClient(client)

	client.logger = &cacheme.NOPLogger{}
	return client
}

func NewCluster(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis, cluster: true}

	client.FixCacheStore = FixCacheStore.Clone(client.redis)
	client.FixCacheStore.SetClient(client)

	client.SimpleCacheStore = SimpleCacheStore.Clone(client.redis)
	client.SimpleCacheStore.SetClient(client)

	client.SimpleMultiCacheStore = SimpleMultiCacheStore.Clone(client.redis)
	client.SimpleMultiCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone(client.redis)
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone(client.redis)
	client.FooCacheStore.SetClient(client)

	client.BarCacheStore = BarCacheStore.Clone(client.redis)
	client.BarCacheStore.SetClient(client)

	client.FooPCacheStore = FooPCacheStore.Clone(client.redis)
	client.FooPCacheStore.SetClient(client)

	client.FooListCacheStore = FooListCacheStore.Clone(client.redis)
	client.FooListCacheStore.SetClient(client)

	client.FooListPCacheStore = FooListPCacheStore.Clone(client.redis)
	client.FooListPCacheStore.SetClient(client)

	client.FooMapSCacheStore = FooMapSCacheStore.Clone(client.redis)
	client.FooMapSCacheStore.SetClient(client)

	client.SimpleFlightCacheStore = SimpleFlightCacheStore.Clone(client.redis)
	client.SimpleFlightCacheStore.SetClient(client)

	client.logger = &cacheme.NOPLogger{}
	return client
}

func (c *Client) NewPipeline() *cacheme.CachePipeline {
	return cacheme.NewPipeline(c.redis)

}

type fixCache struct {
	Fetch       func(ctx context.Context) (string, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FixPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       string
	error        error
	store        *fixCache
	ctx          context.Context
}

func (p *FixPromise) WaitExecute(cp *cacheme.CachePipeline, key string) {
	defer cp.Wg.Done()
	var t string
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
		)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FixPromise) Result() (string, error) {
	return p.result, p.error
}

var FixCacheStore = &fixCache{tag: "Fix"}

func (s *fixCache) SetClient(c *Client) {
	s.client = c
}

func (s *fixCache) Clone(r cacheme.RedisClient) *fixCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fixCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fixCache) KeyTemplate() string {
	return "fix" + ":v" + s.Version()
}

func (s *fixCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fixCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fixCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fixCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fixCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fixCache) Tag() string {
	return s.tag
}

func (s *fixCache) GetP(ctx context.Context, pp *cacheme.CachePipeline) (*FixPromise, error) {
	params := make(map[string]string)

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FixPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key)
	return promise, nil
}

func (s *fixCache) Get(ctx context.Context) (string, error) {

	params := make(map[string]string)

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx)
		})
		return data.(string), err
	}
	return s.get(ctx)
}

func (s *fixCache) get(ctx context.Context) (string, error) {
	params := make(map[string]string)

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fixCache) Update(ctx context.Context) error {

	params := make(map[string]string)

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fixCache) Invalid(ctx context.Context) error {

	params := make(map[string]string)

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fixCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type simpleCache struct {
	Fetch       func(ctx context.Context, ID string) (string, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type SimplePromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       string
	error        error
	store        *simpleCache
	ctx          context.Context
}

func (p *SimplePromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t string
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *SimplePromise) Result() (string, error) {
	return p.result, p.error
}

var SimpleCacheStore = &simpleCache{tag: "Simple"}

func (s *simpleCache) SetClient(c *Client) {
	s.client = c
}

func (s *simpleCache) Clone(r cacheme.RedisClient) *simpleCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *simpleCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *simpleCache) KeyTemplate() string {
	return "simple:{{.ID}}" + ":v" + s.Version()
}

func (s *simpleCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *simpleCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *simpleCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *simpleCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *simpleCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *simpleCache) Tag() string {
	return s.tag
}

func (s *simpleCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*SimplePromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &SimplePromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *simpleCache) Get(ctx context.Context, ID string) (string, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(string), err
	}
	return s.get(ctx, ID)
}

func (s *simpleCache) get(ctx context.Context, ID string) (string, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *simpleCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *simpleCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *simpleCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type simpleMultiCache struct {
	Fetch       func(ctx context.Context, Foo string, Bar string, ID string) (string, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type SimpleMultiPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       string
	error        error
	store        *simpleMultiCache
	ctx          context.Context
}

func (p *SimpleMultiPromise) WaitExecute(cp *cacheme.CachePipeline, key string, Foo string, Bar string, ID string) {
	defer cp.Wg.Done()
	var t string
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			Foo, Bar, ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *SimpleMultiPromise) Result() (string, error) {
	return p.result, p.error
}

var SimpleMultiCacheStore = &simpleMultiCache{tag: "SimpleMulti"}

func (s *simpleMultiCache) SetClient(c *Client) {
	s.client = c
}

func (s *simpleMultiCache) Clone(r cacheme.RedisClient) *simpleMultiCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *simpleMultiCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *simpleMultiCache) KeyTemplate() string {
	return "simplem:{{.Foo}}:{{.Bar}}:{{.ID}}" + ":v" + s.Version()
}

func (s *simpleMultiCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *simpleMultiCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *simpleMultiCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *simpleMultiCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *simpleMultiCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *simpleMultiCache) Tag() string {
	return s.tag
}

func (s *simpleMultiCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, Foo string, Bar string, ID string) (*SimpleMultiPromise, error) {
	params := make(map[string]string)

	params["Foo"] = Foo

	params["Bar"] = Bar

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &SimpleMultiPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, Foo, Bar, ID)
	return promise, nil
}

func (s *simpleMultiCache) Get(ctx context.Context, Foo string, Bar string, ID string) (string, error) {

	params := make(map[string]string)

	params["Foo"] = Foo

	params["Bar"] = Bar

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, Foo, Bar, ID)
		})
		return data.(string), err
	}
	return s.get(ctx, Foo, Bar, ID)
}

func (s *simpleMultiCache) get(ctx context.Context, Foo string, Bar string, ID string) (string, error) {
	params := make(map[string]string)

	params["Foo"] = Foo

	params["Bar"] = Bar

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, Foo, Bar, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *simpleMultiCache) Update(ctx context.Context, Foo string, Bar string, ID string) error {

	params := make(map[string]string)

	params["Foo"] = Foo

	params["Bar"] = Bar

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, Foo, Bar, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *simpleMultiCache) Invalid(ctx context.Context, Foo string, Bar string, ID string) error {

	params := make(map[string]string)

	params["Foo"] = Foo

	params["Bar"] = Bar

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *simpleMultiCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooMapCache struct {
	Fetch       func(ctx context.Context, ID string) (map[string]string, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooMapPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       map[string]string
	error        error
	store        *fooMapCache
	ctx          context.Context
}

func (p *FooMapPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t map[string]string
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooMapPromise) Result() (map[string]string, error) {
	return p.result, p.error
}

var FooMapCacheStore = &fooMapCache{tag: "FooMap"}

func (s *fooMapCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooMapCache) Clone(r cacheme.RedisClient) *fooMapCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooMapCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooMapCache) KeyTemplate() string {
	return "foomap:{{.ID}}" + ":v" + s.Version()
}

func (s *fooMapCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooMapCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooMapCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooMapCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooMapCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooMapCache) Tag() string {
	return s.tag
}

func (s *fooMapCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooMapPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooMapPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooMapCache) Get(ctx context.Context, ID string) (map[string]string, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t map[string]string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(map[string]string), err
	}
	return s.get(ctx, ID)
}

func (s *fooMapCache) get(ctx context.Context, ID string) (map[string]string, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t map[string]string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooMapCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooMapCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooMapCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooCache struct {
	Fetch       func(ctx context.Context, ID string) (model.Foo, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       model.Foo
	error        error
	store        *fooCache
	ctx          context.Context
}

func (p *FooPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t model.Foo
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooPromise) Result() (model.Foo, error) {
	return p.result, p.error
}

var FooCacheStore = &fooCache{tag: "Foo"}

func (s *fooCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooCache) Clone(r cacheme.RedisClient) *fooCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooCache) KeyTemplate() string {
	return "foo:{{.ID}}:info" + ":v" + s.Version()
}

func (s *fooCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooCache) Tag() string {
	return s.tag
}

func (s *fooCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooCache) Get(ctx context.Context, ID string) (model.Foo, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(model.Foo), err
	}
	return s.get(ctx, ID)
}

func (s *fooCache) get(ctx context.Context, ID string) (model.Foo, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type barCache struct {
	Fetch       func(ctx context.Context, ID string) (model.Bar, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type BarPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       model.Bar
	error        error
	store        *barCache
	ctx          context.Context
}

func (p *BarPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t model.Bar
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *BarPromise) Result() (model.Bar, error) {
	return p.result, p.error
}

var BarCacheStore = &barCache{tag: "Bar"}

func (s *barCache) SetClient(c *Client) {
	s.client = c
}

func (s *barCache) Clone(r cacheme.RedisClient) *barCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *barCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *barCache) KeyTemplate() string {
	return "bar:{{.ID}}:info" + ":v" + s.Version()
}

func (s *barCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *barCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *barCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *barCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *barCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *barCache) Tag() string {
	return s.tag
}

func (s *barCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*BarPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &BarPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *barCache) Get(ctx context.Context, ID string) (model.Bar, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t model.Bar

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(model.Bar), err
	}
	return s.get(ctx, ID)
}

func (s *barCache) get(ctx context.Context, ID string) (model.Bar, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t model.Bar

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *barCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *barCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *barCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooPCache struct {
	Fetch       func(ctx context.Context, ID string) (*model.Foo, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooPPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       *model.Foo
	error        error
	store        *fooPCache
	ctx          context.Context
}

func (p *FooPPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t *model.Foo
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooPPromise) Result() (*model.Foo, error) {
	return p.result, p.error
}

var FooPCacheStore = &fooPCache{tag: "FooP"}

func (s *fooPCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooPCache) Clone(r cacheme.RedisClient) *fooPCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooPCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooPCache) KeyTemplate() string {
	return "foop:{{.ID}}:info" + ":v" + s.Version()
}

func (s *fooPCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooPCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooPCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooPCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooPCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooPCache) Tag() string {
	return s.tag
}

func (s *fooPCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooPPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooPPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooPCache) Get(ctx context.Context, ID string) (*model.Foo, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t *model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(*model.Foo), err
	}
	return s.get(ctx, ID)
}

func (s *fooPCache) get(ctx context.Context, ID string) (*model.Foo, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t *model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooPCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooPCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooPCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooListCache struct {
	Fetch       func(ctx context.Context, ID string) ([]model.Foo, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooListPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       []model.Foo
	error        error
	store        *fooListCache
	ctx          context.Context
}

func (p *FooListPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t []model.Foo
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooListPromise) Result() ([]model.Foo, error) {
	return p.result, p.error
}

var FooListCacheStore = &fooListCache{tag: "FooList"}

func (s *fooListCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooListCache) Clone(r cacheme.RedisClient) *fooListCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooListCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooListCache) KeyTemplate() string {
	return "foo:list:{{.ID}}" + ":v" + s.Version()
}

func (s *fooListCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooListCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooListCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooListCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooListCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooListCache) Tag() string {
	return s.tag
}

func (s *fooListCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooListPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooListPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooListCache) Get(ctx context.Context, ID string) ([]model.Foo, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t []model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.([]model.Foo), err
	}
	return s.get(ctx, ID)
}

func (s *fooListCache) get(ctx context.Context, ID string) ([]model.Foo, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t []model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooListCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooListCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooListCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooListPCache struct {
	Fetch       func(ctx context.Context, ID string) ([]*model.Foo, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooListPPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       []*model.Foo
	error        error
	store        *fooListPCache
	ctx          context.Context
}

func (p *FooListPPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t []*model.Foo
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooListPPromise) Result() ([]*model.Foo, error) {
	return p.result, p.error
}

var FooListPCacheStore = &fooListPCache{tag: "FooListP"}

func (s *fooListPCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooListPCache) Clone(r cacheme.RedisClient) *fooListPCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooListPCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooListPCache) KeyTemplate() string {
	return "foo:listp:{{.ID}}" + ":v" + s.Version()
}

func (s *fooListPCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooListPCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooListPCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooListPCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooListPCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooListPCache) Tag() string {
	return s.tag
}

func (s *fooListPCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooListPPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooListPPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooListPCache) Get(ctx context.Context, ID string) ([]*model.Foo, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t []*model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.([]*model.Foo), err
	}
	return s.get(ctx, ID)
}

func (s *fooListPCache) get(ctx context.Context, ID string) ([]*model.Foo, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t []*model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooListPCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooListPCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooListPCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooMapSCache struct {
	Fetch       func(ctx context.Context, ID string) (map[model.Foo]model.Bar, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type FooMapSPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       map[model.Foo]model.Bar
	error        error
	store        *fooMapSCache
	ctx          context.Context
}

func (p *FooMapSPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t map[model.Foo]model.Bar
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if false {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooMapSPromise) Result() (map[model.Foo]model.Bar, error) {
	return p.result, p.error
}

var FooMapSCacheStore = &fooMapSCache{tag: "FooMapS"}

func (s *fooMapSCache) SetClient(c *Client) {
	s.client = c
}

func (s *fooMapSCache) Clone(r cacheme.RedisClient) *fooMapSCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *fooMapSCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *fooMapSCache) KeyTemplate() string {
	return "foo:maps:{{.ID}}" + ":v" + s.Version()
}

func (s *fooMapSCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooMapSCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *fooMapSCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *fooMapSCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooMapSCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooMapSCache) Tag() string {
	return s.tag
}

func (s *fooMapSCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooMapSPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooMapSPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *fooMapSCache) Get(ctx context.Context, ID string) (map[model.Foo]model.Bar, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t map[model.Foo]model.Bar

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if false {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(map[model.Foo]model.Bar), err
	}
	return s.get(ctx, ID)
}

func (s *fooMapSCache) get(ctx context.Context, ID string) (map[model.Foo]model.Bar, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t map[model.Foo]model.Bar

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooMapSCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooMapSCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *fooMapSCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type simpleFlightCache struct {
	Fetch       func(ctx context.Context, ID string) (string, error)
	tag         string
	memo        *cacheme.RedisMemoLock
	client      *Client
	version     string
	versionFunc func() string
}

type SimpleFlightPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       string
	error        error
	store        *simpleFlightCache
	ctx          context.Context
}

func (p *SimpleFlightPromise) WaitExecute(cp *cacheme.CachePipeline, key string, ID string) {
	defer cp.Wg.Done()
	var t string
	memo := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		p.store.client.logger.Log(p.store.tag, key, Hit)
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}
	p.store.client.logger.Log(p.store.tag, key, Miss)

	if resourceLock {
		p.store.client.logger.Log(p.store.tag, key, Fetch)
		value, err := p.store.Fetch(
			p.ctx,
			ID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	var res []byte
	if true {
		res, err = memo.WaitSingle(p.ctx, key)
	} else {
		res, err = memo.Wait(p.ctx, key)
	}
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *SimpleFlightPromise) Result() (string, error) {
	return p.result, p.error
}

var SimpleFlightCacheStore = &simpleFlightCache{tag: "SimpleFlight"}

func (s *simpleFlightCache) SetClient(c *Client) {
	s.client = c
}

func (s *simpleFlightCache) Clone(r cacheme.RedisClient) *simpleFlightCache {
	value := *s
	new := &value
	lock, err := cacheme.NewRedisMemoLock(
		context.TODO(), "cacheme", r, s.tag, 5*time.Second,
	)
	if err != nil {
		fmt.Println(err)
	}
	new.memo = lock

	return new
}

func (s *simpleFlightCache) Version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.version
}

func (s *simpleFlightCache) KeyTemplate() string {
	return "simple:flight:{{.ID}}" + ":v" + s.Version()
}

func (s *simpleFlightCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *simpleFlightCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.Version()
}

func (s *simpleFlightCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *simpleFlightCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *simpleFlightCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *simpleFlightCache) Tag() string {
	return s.tag
}

func (s *simpleFlightCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*SimpleFlightPromise, error) {
	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &SimpleFlightPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, ID)
	return promise, nil
}

func (s *simpleFlightCache) Get(ctx context.Context, ID string) (string, error) {

	params := make(map[string]string)

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	if true {
		data, err, _ := s.memo.SingleGroup().Do(key, func() (interface{}, error) {
			return s.get(ctx, ID)
		})
		return data.(string), err
	}
	return s.get(ctx, ID)
}

func (s *simpleFlightCache) get(ctx context.Context, ID string) (string, error) {
	params := make(map[string]string)

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo
	var res []byte

	res, err = memo.GetCached(ctx, key)
	if err == nil {
		s.client.logger.Log(s.tag, key, Hit)
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}
	s.client.logger.Log(s.tag, key, Miss)

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
		s.client.logger.Log(s.tag, key, Fetch)
		value, err := s.Fetch(ctx, ID)
		if err != nil {
			return value, err
		}
		packed, err := cacheme.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)

	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *simpleFlightCache) Update(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, ID)
	if err != nil {
		return err
	}
	packed, err := cacheme.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *simpleFlightCache) Invalid(ctx context.Context, ID string) error {

	params := make(map[string]string)

	params["ID"] = ID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *simpleFlightCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}
