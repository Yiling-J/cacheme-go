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

	"github.com/Yiling-J/cacheme-go/integration/model"
)

const (
	Hit   = "HIT"
	Miss  = "MISS"
	Fetch = "FETCH"
)

type Client struct {
	SimpleCacheStore *simpleCache

	FooMapCacheStore *fooMapCache

	FooCacheStore *fooCache

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

	client.SimpleCacheStore = SimpleCacheStore.Clone(client.redis)
	client.SimpleCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone(client.redis)
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone(client.redis)
	client.FooCacheStore.SetClient(client)

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

	client.SimpleCacheStore = SimpleCacheStore.Clone(client.redis)
	client.SimpleCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone(client.redis)
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone(client.redis)
	client.FooCacheStore.SetClient(client)

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

var stores = []cacheme.CacheStore{

	SimpleCacheStore,

	FooMapCacheStore,

	FooCacheStore,

	FooPCacheStore,

	FooListCacheStore,

	FooListPCacheStore,

	FooMapSCacheStore,

	SimpleFlightCacheStore,
}

type simpleCache struct {
	Fetch  func(ctx context.Context, ID string) (string, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *simpleCache) KeyTemplate() string {
	return "simple:{{.ID}}" + ":v1"
}

func (s *simpleCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *simpleCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *simpleCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *simpleCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooMapCache struct {
	Fetch  func(ctx context.Context, ID string) (map[string]string, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooMapCache) KeyTemplate() string {
	return "foomap:{{.ID}}" + ":v1"
}

func (s *fooMapCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooMapCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooMapCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooMapCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooCache struct {
	Fetch  func(ctx context.Context, ID string) (model.Foo, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooCache) KeyTemplate() string {
	return "foo:{{.ID}}:info" + ":v1"
}

func (s *fooCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooPCache struct {
	Fetch  func(ctx context.Context, ID string) (*model.Foo, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooPCache) KeyTemplate() string {
	return "foop:{{.ID}}:info" + ":v1"
}

func (s *fooPCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooPCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooPCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooPCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooListCache struct {
	Fetch  func(ctx context.Context, ID string) ([]model.Foo, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooListCache) KeyTemplate() string {
	return "foo:list:{{.ID}}" + ":v1"
}

func (s *fooListCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooListCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooListCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooListCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooListPCache struct {
	Fetch  func(ctx context.Context, ID string) ([]*model.Foo, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooListPCache) KeyTemplate() string {
	return "foo:listp:{{.ID}}" + ":v1"
}

func (s *fooListPCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooListPCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooListPCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooListPCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type fooMapSCache struct {
	Fetch  func(ctx context.Context, ID string) (map[model.Foo]model.Bar, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *fooMapSCache) KeyTemplate() string {
	return "foo:maps:{{.ID}}" + ":v1"
}

func (s *fooMapSCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooMapSCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *fooMapSCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if false {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *fooMapSCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}

type simpleFlightCache struct {
	Fetch  func(ctx context.Context, ID string) (string, error)
	tag    string
	memo   *cacheme.RedisMemoLock
	client *Client
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

	res, err := memo.Wait(p.ctx, key)
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

func (s *simpleFlightCache) KeyTemplate() string {
	return "simple:flight:{{.ID}}" + ":v1"
}

func (s *simpleFlightCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *simpleFlightCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":v1"
}

func (s *simpleFlightCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":v" + strconv.Itoa(v)
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

	memo := s.memo
	var res []byte
	var hitRedis bool
	if true {
		res, hitRedis, err = memo.GetCachedSingle(ctx, key)
	} else {
		hitRedis = true
		res, err = memo.GetCached(ctx, key)
	}
	if err == nil {
		if hitRedis {
			s.client.logger.Log(s.tag, key, Hit)
		}
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

func (s *simpleFlightCache) InvalidAll(ctx context.Context, version int) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}
