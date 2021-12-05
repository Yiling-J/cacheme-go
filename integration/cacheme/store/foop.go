// Code generated by cacheme, DO NOT EDIT.
package store

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

	"github.com/Yiling-J/cacheme-go/integration/model"
)

type FooPCache struct {
	Fetch         func(ctx context.Context, ID string) (*model.Foo, error)
	tag           string
	memo          *cacheme.RedisMemoLock
	client        *Client
	versionString string
	versionFunc   func() string
	singleflight  bool
	metadata      bool
}

type FooPPromise struct {
	executed     chan bool
	redisPromise *redis.StringCmd
	result       *model.Foo
	error        error
	store        *FooPCache
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
			if p.store.metadata {
				memo.AddGroup(p.ctx, p.store.group(), key)
			}
		}
		p.error = err
		return
	}

	var res []byte
	if p.store.singleflight {
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

func (s *FooPCache) setClient(c *Client) {
	s.client = c
}

func (s *FooPCache) clone(r cacheme.RedisClient) *FooPCache {
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

func (s *FooPCache) version() string {
	if s.versionFunc != nil {
		return s.versionFunc()
	}
	return s.versionString
}

func (s *FooPCache) keyTemplate() string {
	return "foop:{{.ID}}:info" + ":v" + s.version()
}

func (s *FooPCache) key(p *fooPParam) (string, error) {
	t := template.Must(template.New("").Parse(s.keyTemplate()))
	t = t.Option("missingkey=zero")
	var tpl bytes.Buffer
	err := t.Execute(&tpl, p)
	return tpl.String(), err
}

func (s *FooPCache) group() string {
	return "cacheme" + ":group:" + s.tag + ":v" + s.version()
}

func (s *FooPCache) versionedGroup(v string) string {
	return "cacheme" + ":group:" + s.tag + ":v" + v
}

func (s *FooPCache) addMemoLock() error {
	lock, err := cacheme.NewRedisMemoLock(context.TODO(), "cacheme", s.client.redis, s.tag, 5*time.Second)
	if err != nil {
		return err
	}

	s.memo = lock
	return nil
}

func (s *FooPCache) initialized() bool {
	return s.Fetch != nil
}

func (s *FooPCache) GetP(ctx context.Context, pp *cacheme.CachePipeline, ID string) (*FooPPromise, error) {
	param := &fooPParam{}

	param.ID = ID

	key, err := s.key(param)
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

func (s *FooPCache) Get(ctx context.Context, ID string) (*model.Foo, error) {

	param := &fooPParam{}

	param.ID = ID

	var t *model.Foo

	key, err := s.key(param)
	if err != nil {
		return t, err
	}

	if s.singleflight {
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
	store *FooPCache
	keys  []fooPParam
}

type FooPQuerySet struct {
	keys    []string
	results map[string]*model.Foo
}

func (q *FooPQuerySet) Get(ID string) (*model.Foo, error) {
	param := fooPParam{

		ID: ID,
	}
	v, ok := q.results[param.pid()]
	if !ok {
		return v, errors.New("not found")
	}
	return v, nil
}

func (q *FooPQuerySet) GetSlice() []*model.Foo {
	var results []*model.Foo
	for _, k := range q.keys {
		results = append(results, q.results[k])
	}
	return results
}

func (s *FooPCache) MGetter() *FooPMultiGetter {
	return &FooPMultiGetter{
		store: s,
		keys:  []fooPParam{},
	}
}

func (g *FooPMultiGetter) GetM(ID string) *FooPMultiGetter {
	g.keys = append(g.keys, fooPParam{ID: ID})
	return g
}

func (g *FooPMultiGetter) Do(ctx context.Context) (*FooPQuerySet, error) {
	qs := &FooPQuerySet{}
	var keys []string
	for _, k := range g.keys {
		pid := k.pid()
		qs.keys = append(qs.keys, pid)
		keys = append(keys, pid)
	}
	if g.store.singleflight {
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

func (s *FooPCache) GetM(ID string) *FooPMultiGetter {
	return &FooPMultiGetter{
		store: s,
		keys:  []fooPParam{{ID: ID}},
	}
}

func (s *FooPCache) get(ctx context.Context, ID string) (*model.Foo, error) {
	param := &fooPParam{}

	param.ID = ID

	var t *model.Foo

	key, err := s.key(param)
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
			if s.metadata {
				memo.AddGroup(ctx, s.group(), key)
			}
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

func (s *FooPCache) Update(ctx context.Context, ID string) error {

	param := &fooPParam{}

	param.ID = ID

	key, err := s.key(param)
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
		if s.metadata {
			s.memo.AddGroup(ctx, s.group(), key)
		}
	}
	return err
}

func (s *FooPCache) Invalid(ctx context.Context, ID string) error {

	param := &fooPParam{}

	param.ID = ID

	key, err := s.key(param)
	if err != nil {
		return err
	}
	return s.memo.DeleteCache(ctx, key)

}

func (s *FooPCache) InvalidAll(ctx context.Context, version string) error {
	group := s.versionedGroup(version)
	if s.client.cluster {
		return cacheme.InvalidAllCluster(ctx, group, s.client.redis)
	}
	return cacheme.InvalidAll(ctx, group, s.client.redis)

}
