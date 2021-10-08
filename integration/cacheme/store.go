//nolint
package cacheme

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"text/template"
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
	"github.com/go-redis/redis/v8"

	"github.com/Yiling-J/cacheme-go/integration/model"
)

type Client struct {
	SimpleCacheStore *simpleCache

	FooMapCacheStore *fooMapCache

	FooCacheStore *fooCache

	FooPCacheStore *fooPCache

	FooListCacheStore *fooListCache

	FooListPCacheStore *fooListPCache

	redis   cacheme.RedisClient
	cluster bool
}

func (c *Client) Redis() cacheme.RedisClient {
	return c.redis
}

func New(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis}

	client.SimpleCacheStore = SimpleCacheStore.Clone()
	client.SimpleCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone()
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone()
	client.FooCacheStore.SetClient(client)

	client.FooPCacheStore = FooPCacheStore.Clone()
	client.FooPCacheStore.SetClient(client)

	client.FooListCacheStore = FooListCacheStore.Clone()
	client.FooListCacheStore.SetClient(client)

	client.FooListPCacheStore = FooListPCacheStore.Clone()
	client.FooListPCacheStore.SetClient(client)

	return client
}

func NewCluster(redis cacheme.RedisClient) *Client {
	client := &Client{redis: redis, cluster: true}

	client.SimpleCacheStore = SimpleCacheStore.Clone()
	client.SimpleCacheStore.SetClient(client)

	client.FooMapCacheStore = FooMapCacheStore.Clone()
	client.FooMapCacheStore.SetClient(client)

	client.FooCacheStore = FooCacheStore.Clone()
	client.FooCacheStore.SetClient(client)

	client.FooPCacheStore = FooPCacheStore.Clone()
	client.FooPCacheStore.SetClient(client)

	client.FooListCacheStore = FooListCacheStore.Clone()
	client.FooListCacheStore.SetClient(client)

	client.FooListPCacheStore = FooListPCacheStore.Clone()
	client.FooListPCacheStore.SetClient(client)

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
}

type simpleCache struct {
	Fetch  func(ctx context.Context, ID string) (string, error)
	tag    string
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *simpleCache) Clone() *simpleCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *fooMapCache) Clone() *fooMapCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t map[string]string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *fooCache) Clone() *fooCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *fooPCache) Clone() *fooPCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t *model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *fooListCache) Clone() *fooListCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t []model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
	once   sync.Once
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
		err = cacheme.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := memo.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
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

func (s *fooListPCache) Clone() *fooListPCache {
	new := *s
	return &new
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

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["ID"] = ID

	var t []*model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = cacheme.Unmarshal(res, &t)
		return t, err
	}

	if err != redis.Nil {
		return t, errors.New("")
	}

	resourceLock, err := memo.Lock(ctx, key)
	if err != nil {
		return t, err
	}

	if resourceLock {
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
