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

	"github.com/Yiling-J/cacheme-go/cacheme"
	"github.com/go-redis/redis/v8"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/Yiling-J/cacheme-go/integration/model"
)

type Client struct {
	FooBarCacheStore *fooBarCache

	FooOneCacheStore *fooOneCache

	FooListCacheStore *fooListCache

	redis *redis.Client
}

func New(redis *redis.Client) *Client {
	client := Client{redis: redis}

	client.FooBarCacheStore = FooBarCacheStore.Clone()
	client.FooBarCacheStore.SetClient(redis)

	client.FooOneCacheStore = FooOneCacheStore.Clone()
	client.FooOneCacheStore.SetClient(redis)

	client.FooListCacheStore = FooListCacheStore.Clone()
	client.FooListCacheStore.SetClient(redis)

	return &client
}

func (c *Client) NewPipeline() *cacheme.CachePipeline {
	return cacheme.NewPipeline(c.redis)

}

var stores = []cacheme.CacheStore{

	FooBarCacheStore,

	FooOneCacheStore,

	FooListCacheStore,
}

type fooBarCache struct {
	Fetch func(ctx context.Context, fooID string, barID string) (string, error)
	tag   string
	once  sync.Once
	memo  *cacheme.RedisMemoLock
	redis *redis.Client
}

type FooBarPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       string
	error        error
	store        *fooBarCache
	ctx          context.Context
}

func (p *FooBarPromise) WaitExecute(cp *cacheme.CachePipeline, key string, fooID string, barID string) {
	defer cp.Wg.Done()
	var t string
	cacheme := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		err = msgpack.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := cacheme.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
		value, err := p.store.Fetch(
			p.ctx,
			fooID, barID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := msgpack.Marshal(value)
		if err == nil {
			cacheme.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			cacheme.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	res, err := cacheme.Wait(p.ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooBarPromise) Result() (string, error) {
	return p.result, p.error
}

var FooBarCacheStore = &fooBarCache{tag: "FooBar"}

func (s *fooBarCache) SetClient(c *redis.Client) {
	s.redis = c
}

func (s *fooBarCache) Clone() *fooBarCache {
	new := *s
	return &new
}

func (s *fooBarCache) KeyTemplate() string {
	return "foo:{{.fooID}}:bar:{{.barID}}" + ":1"
}

func (s *fooBarCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooBarCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":1"
}

func (s *fooBarCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":" + strconv.Itoa(v)
}

func (s *fooBarCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooBarCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooBarCache) Tag() string {
	return s.tag
}

func (s *fooBarCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, fooID string, barID string) (*FooBarPromise, error) {
	params := make(map[string]string)

	params["fooID"] = fooID

	params["barID"] = barID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooBarPromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, fooID, barID)
	return promise, nil
}

func (s *fooBarCache) Get(ctx context.Context, fooID string, barID string) (string, error) {

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["fooID"] = fooID

	params["barID"] = barID

	var t string

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
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
		value, err := s.Fetch(ctx, fooID, barID)
		if err != nil {
			return value, err
		}
		packed, err := msgpack.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooBarCache) Update(ctx context.Context, fooID string, barID string) error {

	params := make(map[string]string)

	params["fooID"] = fooID

	params["barID"] = barID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, fooID, barID)
	if err != nil {
		return err
	}
	packed, err := msgpack.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooBarCache) Invalid(ctx context.Context, fooID string, barID string) error {

	params := make(map[string]string)

	params["fooID"] = fooID

	params["barID"] = barID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.redis.Del(ctx, key).Err()

}

func (s *fooBarCache) InvalidAll(ctx context.Context, version int) error {

	group := s.versionedGroup(version)
	iter := s.redis.SScan(ctx, group, 0, "", 200).Iterator()
	invalids := []string{}
	for iter.Next(ctx) {
		invalids = append(invalids, iter.Val())
		if len(invalids) == 600 {
			err := s.redis.Unlink(ctx, invalids...).Err()
			if err != nil {
				fmt.Println(err)
			}
			invalids = []string{}
		}
	}

	if len(invalids) > 0 {
		err := s.redis.Unlink(ctx, invalids...).Err()
		if err != nil {
			return err
		}
		err = s.redis.Unlink(ctx, group).Err()
		return err
	}
	return nil
}

type fooOneCache struct {
	Fetch func(ctx context.Context, fooID string) (*model.Foo, error)
	tag   string
	once  sync.Once
	memo  *cacheme.RedisMemoLock
	redis *redis.Client
}

type FooOnePromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       *model.Foo
	error        error
	store        *fooOneCache
	ctx          context.Context
}

func (p *FooOnePromise) WaitExecute(cp *cacheme.CachePipeline, key string, fooID string) {
	defer cp.Wg.Done()
	var t *model.Foo
	cacheme := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		err = msgpack.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := cacheme.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
		value, err := p.store.Fetch(
			p.ctx,
			fooID)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := msgpack.Marshal(value)
		if err == nil {
			cacheme.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			cacheme.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	res, err := cacheme.Wait(p.ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooOnePromise) Result() (*model.Foo, error) {
	return p.result, p.error
}

var FooOneCacheStore = &fooOneCache{tag: "FooOne"}

func (s *fooOneCache) SetClient(c *redis.Client) {
	s.redis = c
}

func (s *fooOneCache) Clone() *fooOneCache {
	new := *s
	return &new
}

func (s *fooOneCache) KeyTemplate() string {
	return "foo:{{.fooID}}:info" + ":1"
}

func (s *fooOneCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooOneCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":1"
}

func (s *fooOneCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":" + strconv.Itoa(v)
}

func (s *fooOneCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *fooOneCache) Initialized() bool {
	return s.Fetch != nil
}

func (s *fooOneCache) Tag() string {
	return s.tag
}

func (s *fooOneCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, fooID string) (*FooOnePromise, error) {
	params := make(map[string]string)

	params["fooID"] = fooID

	key, err := s.Key(params)
	if err != nil {
		return nil, err
	}

	cacheme := s.memo

	promise := &FooOnePromise{
		executed: pp.Executed,
		ctx:      ctx,
		store:    s,
	}

	wait := cacheme.GetCachedP(ctx, pp.Pipeline, key)
	promise.redisPromise = wait
	pp.Wg.Add(1)
	go promise.WaitExecute(
		pp, key, fooID)
	return promise, nil
}

func (s *fooOneCache) Get(ctx context.Context, fooID string) (*model.Foo, error) {

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	params["fooID"] = fooID

	var t *model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
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
		value, err := s.Fetch(ctx, fooID)
		if err != nil {
			return value, err
		}
		packed, err := msgpack.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooOneCache) Update(ctx context.Context, fooID string) error {

	params := make(map[string]string)

	params["fooID"] = fooID

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx, fooID)
	if err != nil {
		return err
	}
	packed, err := msgpack.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooOneCache) Invalid(ctx context.Context, fooID string) error {

	params := make(map[string]string)

	params["fooID"] = fooID

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.redis.Del(ctx, key).Err()

}

func (s *fooOneCache) InvalidAll(ctx context.Context, version int) error {

	group := s.versionedGroup(version)
	iter := s.redis.SScan(ctx, group, 0, "", 200).Iterator()
	invalids := []string{}
	for iter.Next(ctx) {
		invalids = append(invalids, iter.Val())
		if len(invalids) == 600 {
			err := s.redis.Unlink(ctx, invalids...).Err()
			if err != nil {
				fmt.Println(err)
			}
			invalids = []string{}
		}
	}

	if len(invalids) > 0 {
		err := s.redis.Unlink(ctx, invalids...).Err()
		if err != nil {
			return err
		}
		err = s.redis.Unlink(ctx, group).Err()
		return err
	}
	return nil
}

type fooListCache struct {
	Fetch func(ctx context.Context) ([]*model.Foo, error)
	tag   string
	once  sync.Once
	memo  *cacheme.RedisMemoLock
	redis *redis.Client
}

type FooListPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       []*model.Foo
	error        error
	store        *fooListCache
	ctx          context.Context
}

func (p *FooListPromise) WaitExecute(cp *cacheme.CachePipeline, key string) {
	defer cp.Wg.Done()
	var t []*model.Foo
	cacheme := p.store.memo

	<-cp.Executed
	value, err := p.redisPromise.Bytes()
	if err == nil {
		err = msgpack.Unmarshal(value, &t)
		p.result, p.error = t, err
		return
	}

	resourceLock, err := cacheme.Lock(p.ctx, key)
	if err != nil {
		p.error = err
		return
	}

	if resourceLock {
		value, err := p.store.Fetch(
			p.ctx,
		)
		if err != nil {
			p.error = err
			return
		}
		p.result = value
		packed, err := msgpack.Marshal(value)
		if err == nil {
			cacheme.SetCache(p.ctx, key, packed, time.Millisecond*300000)
			cacheme.AddGroup(p.ctx, p.store.Group(), key)
		}
		p.error = err
		return
	}

	res, err := cacheme.Wait(p.ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
	}
	p.result, p.error = t, err
}

func (p *FooListPromise) Result() ([]*model.Foo, error) {
	return p.result, p.error
}

var FooListCacheStore = &fooListCache{tag: "FooList"}

func (s *fooListCache) SetClient(c *redis.Client) {
	s.redis = c
}

func (s *fooListCache) Clone() *fooListCache {
	new := *s
	return &new
}

func (s *fooListCache) KeyTemplate() string {
	return "foo:list" + ":1"
}

func (s *fooListCache) Key(m map[string]string) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, m)
	return tpl.String(), err
}

func (s *fooListCache) Group() string {
	return "cacheme" + ":group:" + s.tag + ":1"
}

func (s *fooListCache) versionedGroup(v int) string {
	return "cacheme" + ":group:" + s.tag + ":" + strconv.Itoa(v)
}

func (s *fooListCache) AddMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
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

func (s *fooListCache) GetP(ctx context.Context, pp *cacheme.CachePipeline) (*FooListPromise, error) {
	params := make(map[string]string)

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
		pp, key)
	return promise, nil
}

func (s *fooListCache) Get(ctx context.Context) ([]*model.Foo, error) {

	s.once.Do(func() {
		lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.redis, s.tag, 5*time.Second)
		if err != nil {
			fmt.Println(err)
		}

		s.memo = lock
	})

	params := make(map[string]string)

	var t []*model.Foo

	key, err := s.Key(params)
	if err != nil {
		return t, err
	}

	memo := s.memo

	res, err := memo.GetCached(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
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
		value, err := s.Fetch(ctx)
		if err != nil {
			return value, err
		}
		packed, err := msgpack.Marshal(value)
		if err == nil {
			memo.SetCache(ctx, key, packed, time.Millisecond*300000)
			memo.AddGroup(ctx, s.Group(), key)
		}
		return value, err
	}

	res, err = memo.Wait(ctx, key)
	if err == nil {
		err = msgpack.Unmarshal(res, &t)
		return t, err
	}
	return t, err
}

func (s *fooListCache) Update(ctx context.Context) error {

	params := make(map[string]string)

	key, err := s.Key(params)
	if err != nil {
		return err
	}

	value, err := s.Fetch(ctx)
	if err != nil {
		return err
	}
	packed, err := msgpack.Marshal(value)
	if err == nil {
		s.memo.SetCache(ctx, key, packed, time.Millisecond*300000)
		s.memo.AddGroup(ctx, s.Group(), key)
	}
	return err
}

func (s *fooListCache) Invalid(ctx context.Context) error {

	params := make(map[string]string)

	key, err := s.Key(params)
	if err != nil {
		return err
	}
	return s.redis.Del(ctx, key).Err()

}

func (s *fooListCache) InvalidAll(ctx context.Context, version int) error {

	group := s.versionedGroup(version)
	iter := s.redis.SScan(ctx, group, 0, "", 200).Iterator()
	invalids := []string{}
	for iter.Next(ctx) {
		invalids = append(invalids, iter.Val())
		if len(invalids) == 600 {
			err := s.redis.Unlink(ctx, invalids...).Err()
			if err != nil {
				fmt.Println(err)
			}
			invalids = []string{}
		}
	}

	if len(invalids) > 0 {
		err := s.redis.Unlink(ctx, invalids...).Err()
		if err != nil {
			return err
		}
		err = s.redis.Unlink(ctx, group).Err()
		return err
	}
	return nil
}
