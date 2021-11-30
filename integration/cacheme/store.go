//nolint
package cacheme

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
	"github.com/go-redis/redis/v8"

	"github.com/Yiling-J/cacheme-go/integration/cacheme/schema"

	"strconv"

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

func (s *fixCache) Key(p *fixParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fixCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fixCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fixParam{}

	key, err := s.Key(param)
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

	param := &fixParam{}

	var t string

	key, err := s.Key(param)
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

type fixParam struct {
}

func (p *fixParam) pid() string {
	var id string

	return id
}

type FixMultiGetter struct {
	store *fixCache
	keys  []fixParam
}

type fixQuerySet struct {
	keys    []string
	results map[string]string
}

func (q *fixQuerySet) Get() (string, error) {
	param := fixParam{}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fixQuerySet) GetSlice() []string {
	var results []string
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fixCache) MGetter() *FixMultiGetter {
	return &FixMultiGetter{
		store: s,
		keys:  []fixParam{},
	}
}

func (g *FixMultiGetter) GetM() *FixMultiGetter {
	g.keys = append(g.keys, fixParam{})
	return g
}

func (g *FixMultiGetter) Do(ctx context.Context) (*fixQuerySet, error) {
	qs := &fixQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]string)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FixMultiGetter) pipeDo(ctx context.Context) (map[string]string, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FixPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fixCache) GetM() *FixMultiGetter {
	return &FixMultiGetter{
		store: s,
		keys:  []fixParam{{}},
	}
}

func (s *fixCache) get(ctx context.Context) (string, error) {
	param := &fixParam{}

	var t string

	key, err := s.Key(param)
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

	param := &fixParam{}

	key, err := s.Key(param)
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

	param := &fixParam{}

	key, err := s.Key(param)
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

func (s *simpleCache) Key(p *simpleParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *simpleCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *simpleCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &simpleParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleParam{}

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

type simpleParam struct {
	ID string
}

func (p *simpleParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type SimpleMultiGetter struct {
	store *simpleCache
	keys  []simpleParam
}

type simpleQuerySet struct {
	keys    []string
	results map[string]string
}

func (q *simpleQuerySet) Get(ID string) (string, error) {
	param := simpleParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *simpleQuerySet) GetSlice() []string {
	var results []string
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *simpleCache) MGetter() *SimpleMultiGetter {
	return &SimpleMultiGetter{
		store: s,
		keys:  []simpleParam{},
	}
}

func (g *SimpleMultiGetter) GetM(ID string) *SimpleMultiGetter {
	g.keys = append(g.keys, simpleParam{ID: ID})
	return g
}

func (g *SimpleMultiGetter) Do(ctx context.Context) (*simpleQuerySet, error) {
	qs := &simpleQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]string)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *SimpleMultiGetter) pipeDo(ctx context.Context) (map[string]string, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*SimplePromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *simpleCache) GetM(ID string) *SimpleMultiGetter {
	return &SimpleMultiGetter{
		store: s,
		keys:  []simpleParam{{ID: ID}},
	}
}

func (s *simpleCache) get(ctx context.Context, ID string) (string, error) {
	param := &simpleParam{}

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

	param := &simpleParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *simpleMultiCache) Key(p *simpleMultiParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *simpleMultiCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *simpleMultiCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &simpleMultiParam{}

	param.Foo = Foo

	param.Bar = Bar

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleMultiParam{}

	param.Foo = Foo

	param.Bar = Bar

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

type simpleMultiParam struct {
	Foo string

	Bar string

	ID string
}

func (p *simpleMultiParam) pid() string {
	var id string

	id = id + p.Foo

	id = id + ":" + p.Bar

	id = id + ":" + p.ID

	return id
}

type SimpleMultiMultiGetter struct {
	store *simpleMultiCache
	keys  []simpleMultiParam
}

type simpleMultiQuerySet struct {
	keys    []string
	results map[string]string
}

func (q *simpleMultiQuerySet) Get(Foo string, Bar string, ID string) (string, error) {
	param := simpleMultiParam{

		Foo: Foo,

		Bar: Bar,

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *simpleMultiQuerySet) GetSlice() []string {
	var results []string
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *simpleMultiCache) MGetter() *SimpleMultiMultiGetter {
	return &SimpleMultiMultiGetter{
		store: s,
		keys:  []simpleMultiParam{},
	}
}

func (g *SimpleMultiMultiGetter) GetM(Foo string, Bar string, ID string) *SimpleMultiMultiGetter {
	g.keys = append(g.keys, simpleMultiParam{Foo: Foo, Bar: Bar, ID: ID})
	return g
}

func (g *SimpleMultiMultiGetter) Do(ctx context.Context) (*simpleMultiQuerySet, error) {
	qs := &simpleMultiQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]string)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *SimpleMultiMultiGetter) pipeDo(ctx context.Context) (map[string]string, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*SimpleMultiPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.Foo, k.Bar, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *simpleMultiCache) GetM(Foo string, Bar string, ID string) *SimpleMultiMultiGetter {
	return &SimpleMultiMultiGetter{
		store: s,
		keys:  []simpleMultiParam{{Foo: Foo, Bar: Bar, ID: ID}},
	}
}

func (s *simpleMultiCache) get(ctx context.Context, Foo string, Bar string, ID string) (string, error) {
	param := &simpleMultiParam{}

	param.Foo = Foo

	param.Bar = Bar

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

	param := &simpleMultiParam{}

	param.Foo = Foo

	param.Bar = Bar

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleMultiParam{}

	param.Foo = Foo

	param.Bar = Bar

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooMapCache) Key(p *fooMapParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooMapCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooMapCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooMapParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooMapParam{}

	param.ID = ID

	var t map[string]string

	key, err := s.Key(param)
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

type fooMapParam struct {
	ID string
}

func (p *fooMapParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooMapMultiGetter struct {
	store *fooMapCache
	keys  []fooMapParam
}

type fooMapQuerySet struct {
	keys    []string
	results map[string]map[string]string
}

func (q *fooMapQuerySet) Get(ID string) (map[string]string, error) {
	param := fooMapParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooMapQuerySet) GetSlice() []map[string]string {
	var results []map[string]string
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooMapCache) MGetter() *FooMapMultiGetter {
	return &FooMapMultiGetter{
		store: s,
		keys:  []fooMapParam{},
	}
}

func (g *FooMapMultiGetter) GetM(ID string) *FooMapMultiGetter {
	g.keys = append(g.keys, fooMapParam{ID: ID})
	return g
}

func (g *FooMapMultiGetter) Do(ctx context.Context) (*fooMapQuerySet, error) {
	qs := &fooMapQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]map[string]string)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooMapMultiGetter) pipeDo(ctx context.Context) (map[string]map[string]string, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooMapPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]map[string]string)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooMapCache) GetM(ID string) *FooMapMultiGetter {
	return &FooMapMultiGetter{
		store: s,
		keys:  []fooMapParam{{ID: ID}},
	}
}

func (s *fooMapCache) get(ctx context.Context, ID string) (map[string]string, error) {
	param := &fooMapParam{}

	param.ID = ID

	var t map[string]string

	key, err := s.Key(param)
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

	param := &fooMapParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooMapParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooCache) Key(p *fooParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooParam{}

	param.ID = ID

	var t model.Foo

	key, err := s.Key(param)
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

type fooParam struct {
	ID string
}

func (p *fooParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooMultiGetter struct {
	store *fooCache
	keys  []fooParam
}

type fooQuerySet struct {
	keys    []string
	results map[string]model.Foo
}

func (q *fooQuerySet) Get(ID string) (model.Foo, error) {
	param := fooParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooQuerySet) GetSlice() []model.Foo {
	var results []model.Foo
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooCache) MGetter() *FooMultiGetter {
	return &FooMultiGetter{
		store: s,
		keys:  []fooParam{},
	}
}

func (g *FooMultiGetter) GetM(ID string) *FooMultiGetter {
	g.keys = append(g.keys, fooParam{ID: ID})
	return g
}

func (g *FooMultiGetter) Do(ctx context.Context) (*fooQuerySet, error) {
	qs := &fooQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]model.Foo)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooMultiGetter) pipeDo(ctx context.Context) (map[string]model.Foo, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]model.Foo)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooCache) GetM(ID string) *FooMultiGetter {
	return &FooMultiGetter{
		store: s,
		keys:  []fooParam{{ID: ID}},
	}
}

func (s *fooCache) get(ctx context.Context, ID string) (model.Foo, error) {
	param := &fooParam{}

	param.ID = ID

	var t model.Foo

	key, err := s.Key(param)
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

	param := &fooParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *barCache) Key(p *barParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *barCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *barCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &barParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &barParam{}

	param.ID = ID

	var t model.Bar

	key, err := s.Key(param)
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

type barParam struct {
	ID string
}

func (p *barParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type BarMultiGetter struct {
	store *barCache
	keys  []barParam
}

type barQuerySet struct {
	keys    []string
	results map[string]model.Bar
}

func (q *barQuerySet) Get(ID string) (model.Bar, error) {
	param := barParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *barQuerySet) GetSlice() []model.Bar {
	var results []model.Bar
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *barCache) MGetter() *BarMultiGetter {
	return &BarMultiGetter{
		store: s,
		keys:  []barParam{},
	}
}

func (g *BarMultiGetter) GetM(ID string) *BarMultiGetter {
	g.keys = append(g.keys, barParam{ID: ID})
	return g
}

func (g *BarMultiGetter) Do(ctx context.Context) (*barQuerySet, error) {
	qs := &barQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]model.Bar)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *BarMultiGetter) pipeDo(ctx context.Context) (map[string]model.Bar, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*BarPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]model.Bar)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *barCache) GetM(ID string) *BarMultiGetter {
	return &BarMultiGetter{
		store: s,
		keys:  []barParam{{ID: ID}},
	}
}

func (s *barCache) get(ctx context.Context, ID string) (model.Bar, error) {
	param := &barParam{}

	param.ID = ID

	var t model.Bar

	key, err := s.Key(param)
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

	param := &barParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &barParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooPCache) Key(p *fooPParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooPCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooPCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooPParam{}

	param.ID = ID

	var t *model.Foo

	key, err := s.Key(param)
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

type fooPParam struct {
	ID string
}

func (p *fooPParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooPMultiGetter struct {
	store *fooPCache
	keys  []fooPParam
}

type fooPQuerySet struct {
	keys    []string
	results map[string]*model.Foo
}

func (q *fooPQuerySet) Get(ID string) (*model.Foo, error) {
	param := fooPParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooPQuerySet) GetSlice() []*model.Foo {
	var results []*model.Foo
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooPCache) MGetter() *FooPMultiGetter {
	return &FooPMultiGetter{
		store: s,
		keys:  []fooPParam{},
	}
}

func (g *FooPMultiGetter) GetM(ID string) *FooPMultiGetter {
	g.keys = append(g.keys, fooPParam{ID: ID})
	return g
}

func (g *FooPMultiGetter) Do(ctx context.Context) (*fooPQuerySet, error) {
	qs := &fooPQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]*model.Foo)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooPMultiGetter) pipeDo(ctx context.Context) (map[string]*model.Foo, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooPPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]*model.Foo)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooPCache) GetM(ID string) *FooPMultiGetter {
	return &FooPMultiGetter{
		store: s,
		keys:  []fooPParam{{ID: ID}},
	}
}

func (s *fooPCache) get(ctx context.Context, ID string) (*model.Foo, error) {
	param := &fooPParam{}

	param.ID = ID

	var t *model.Foo

	key, err := s.Key(param)
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

	param := &fooPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooListCache) Key(p *fooListParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooListCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooListCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooListParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooListParam{}

	param.ID = ID

	var t []model.Foo

	key, err := s.Key(param)
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

type fooListParam struct {
	ID string
}

func (p *fooListParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooListMultiGetter struct {
	store *fooListCache
	keys  []fooListParam
}

type fooListQuerySet struct {
	keys    []string
	results map[string][]model.Foo
}

func (q *fooListQuerySet) Get(ID string) ([]model.Foo, error) {
	param := fooListParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooListQuerySet) GetSlice() [][]model.Foo {
	var results [][]model.Foo
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooListCache) MGetter() *FooListMultiGetter {
	return &FooListMultiGetter{
		store: s,
		keys:  []fooListParam{},
	}
}

func (g *FooListMultiGetter) GetM(ID string) *FooListMultiGetter {
	g.keys = append(g.keys, fooListParam{ID: ID})
	return g
}

func (g *FooListMultiGetter) Do(ctx context.Context) (*fooListQuerySet, error) {
	qs := &fooListQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string][]model.Foo)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooListMultiGetter) pipeDo(ctx context.Context) (map[string][]model.Foo, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooListPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string][]model.Foo)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooListCache) GetM(ID string) *FooListMultiGetter {
	return &FooListMultiGetter{
		store: s,
		keys:  []fooListParam{{ID: ID}},
	}
}

func (s *fooListCache) get(ctx context.Context, ID string) ([]model.Foo, error) {
	param := &fooListParam{}

	param.ID = ID

	var t []model.Foo

	key, err := s.Key(param)
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

	param := &fooListParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooListParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooListPCache) Key(p *fooListPParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooListPCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooListPCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooListPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooListPParam{}

	param.ID = ID

	var t []*model.Foo

	key, err := s.Key(param)
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

type fooListPParam struct {
	ID string
}

func (p *fooListPParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooListPMultiGetter struct {
	store *fooListPCache
	keys  []fooListPParam
}

type fooListPQuerySet struct {
	keys    []string
	results map[string][]*model.Foo
}

func (q *fooListPQuerySet) Get(ID string) ([]*model.Foo, error) {
	param := fooListPParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooListPQuerySet) GetSlice() [][]*model.Foo {
	var results [][]*model.Foo
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooListPCache) MGetter() *FooListPMultiGetter {
	return &FooListPMultiGetter{
		store: s,
		keys:  []fooListPParam{},
	}
}

func (g *FooListPMultiGetter) GetM(ID string) *FooListPMultiGetter {
	g.keys = append(g.keys, fooListPParam{ID: ID})
	return g
}

func (g *FooListPMultiGetter) Do(ctx context.Context) (*fooListPQuerySet, error) {
	qs := &fooListPQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string][]*model.Foo)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooListPMultiGetter) pipeDo(ctx context.Context) (map[string][]*model.Foo, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooListPPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string][]*model.Foo)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooListPCache) GetM(ID string) *FooListPMultiGetter {
	return &FooListPMultiGetter{
		store: s,
		keys:  []fooListPParam{{ID: ID}},
	}
}

func (s *fooListPCache) get(ctx context.Context, ID string) ([]*model.Foo, error) {
	param := &fooListPParam{}

	param.ID = ID

	var t []*model.Foo

	key, err := s.Key(param)
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

	param := &fooListPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooListPParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *fooMapSCache) Key(p *fooMapSParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *fooMapSCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *fooMapSCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &fooMapSParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooMapSParam{}

	param.ID = ID

	var t map[model.Foo]model.Bar

	key, err := s.Key(param)
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

type fooMapSParam struct {
	ID string
}

func (p *fooMapSParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type FooMapSMultiGetter struct {
	store *fooMapSCache
	keys  []fooMapSParam
}

type fooMapSQuerySet struct {
	keys    []string
	results map[string]map[model.Foo]model.Bar
}

func (q *fooMapSQuerySet) Get(ID string) (map[model.Foo]model.Bar, error) {
	param := fooMapSParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *fooMapSQuerySet) GetSlice() []map[model.Foo]model.Bar {
	var results []map[model.Foo]model.Bar
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *fooMapSCache) MGetter() *FooMapSMultiGetter {
	return &FooMapSMultiGetter{
		store: s,
		keys:  []fooMapSParam{},
	}
}

func (g *FooMapSMultiGetter) GetM(ID string) *FooMapSMultiGetter {
	g.keys = append(g.keys, fooMapSParam{ID: ID})
	return g
}

func (g *FooMapSMultiGetter) Do(ctx context.Context) (*fooMapSQuerySet, error) {
	qs := &fooMapSQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if false {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]map[model.Foo]model.Bar)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *FooMapSMultiGetter) pipeDo(ctx context.Context) (map[string]map[model.Foo]model.Bar, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*FooMapSPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]map[model.Foo]model.Bar)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *fooMapSCache) GetM(ID string) *FooMapSMultiGetter {
	return &FooMapSMultiGetter{
		store: s,
		keys:  []fooMapSParam{{ID: ID}},
	}
}

func (s *fooMapSCache) get(ctx context.Context, ID string) (map[model.Foo]model.Bar, error) {
	param := &fooMapSParam{}

	param.ID = ID

	var t map[model.Foo]model.Bar

	key, err := s.Key(param)
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

	param := &fooMapSParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &fooMapSParam{}

	param.ID = ID

	key, err := s.Key(param)
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

func (s *simpleFlightCache) Key(p *simpleFlightParam) (string, error) {
	t := template.Must(template.New("").Parse(s.KeyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *simpleFlightCache) Group() string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + s.Version()
}

func (s *simpleFlightCache) versionedGroup(v string) string {
	return "cacheme" + ":meta:group:" + s.tag + ":v" + v
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
	param := &simpleFlightParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleFlightParam{}

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

type simpleFlightParam struct {
	ID string
}

func (p *simpleFlightParam) pid() string {
	var id string

	id = id + p.ID

	return id
}

type SimpleFlightMultiGetter struct {
	store *simpleFlightCache
	keys  []simpleFlightParam
}

type simpleFlightQuerySet struct {
	keys    []string
	results map[string]string
}

func (q *simpleFlightQuerySet) Get(ID string) (string, error) {
	param := simpleFlightParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *simpleFlightQuerySet) GetSlice() []string {
	var results []string
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *simpleFlightCache) MGetter() *SimpleFlightMultiGetter {
	return &SimpleFlightMultiGetter{
		store: s,
		keys:  []simpleFlightParam{},
	}
}

func (g *SimpleFlightMultiGetter) GetM(ID string) *SimpleFlightMultiGetter {
	g.keys = append(g.keys, simpleFlightParam{ID: ID})
	return g
}

func (g *SimpleFlightMultiGetter) Do(ctx context.Context) (*simpleFlightQuerySet, error) {
	qs := &simpleFlightQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if true {
		sort.Strings(keys)
		group := strings.Join(keys, ":")
		data, err, _ := g.store.memo.SingleGroup().Do(group, func() (interface{}, error) {
			return g.pipeDo(ctx)
		})
		qs.results = data.(map[string]string)
		return qs, err
	}
	data, err := g.pipeDo(ctx)
	qs.results = data
	return qs, err
}

func (g *SimpleFlightMultiGetter) pipeDo(ctx context.Context) (map[string]string, error) {
	pipeline := cacheme.NewPipeline(g.store.client.Redis())
	ps := make(map[string]*SimpleFlightPromise)
	for _, k := range g.keys {
		pid := k.pid()
		if _, ok := ps[pid]; ok {
			continue
		}
		promise, err := g.store.GetP(ctx, pipeline, k.ID)
		if err != nil {
			return nil, err
		}
		ps[pid] = promise
	}

	err := pipeline.Execute(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string)
	for k, p := range ps {
		r, err := p.Result()
		if err != nil {
			return nil, err
		}
		results[k] = r
	}
	return results, nil
}

func (s *simpleFlightCache) GetM(ID string) *SimpleFlightMultiGetter {
	return &SimpleFlightMultiGetter{
		store: s,
		keys:  []simpleFlightParam{{ID: ID}},
	}
}

func (s *simpleFlightCache) get(ctx context.Context, ID string) (string, error) {
	param := &simpleFlightParam{}

	param.ID = ID

	var t string

	key, err := s.Key(param)
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

	param := &simpleFlightParam{}

	param.ID = ID

	key, err := s.Key(param)
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

	param := &simpleFlightParam{}

	param.ID = ID

	key, err := s.Key(param)
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
