package cacheme

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/vmihailenco/msgpack/v5"
)

//go:embed template/*
var templateDir embed.FS

var varRegex = regexp.MustCompile(`{{.([a-zA-Z0-9]+)}}`)

type RedisClient interface {
	PSubscribe(ctx context.Context, channels ...string) *redis.PubSub
	redis.Cmdable
}

type CacheStore interface {
	Initialized() bool
	Tag() string
	AddMemoLock() error
}

func Check(stores []CacheStore) {
	for _, s := range stores {
		if !s.Initialized() {
			fmt.Println(s.Tag() + "has no fetch function")
		}
	}
}

func UpdateMemoLockAll(stores []CacheStore) error {
	for _, s := range stores {
		err := s.AddMemoLock()
		if err != nil {
			return err
		}
	}
	return nil
}

type CachePipeline struct {
	Pipeline redis.Pipeliner
	Wg       *sync.WaitGroup
	Executed chan bool
}

func (p *CachePipeline) Execute(ctx context.Context) error {
	_, err := p.Pipeline.Exec(ctx)
	if err != nil {
		fmt.Println(err)
	}
	close(p.Executed)

	p.Wg.Wait()
	return nil
}

func NewPipeline(client RedisClient) *CachePipeline {
	return &CachePipeline{
		Pipeline: client.Pipeline(),
		Wg:       &sync.WaitGroup{},
		Executed: make(chan bool),
	}

}

type StoreTemplate struct {
	Name    string
	Key     string
	To      string
	Version int
	Vars    []string
	TTL     time.Duration
}

func (s *StoreTemplate) ToType() string {
	return s.To
}

func (s *StoreTemplate) SetVars(a []string) {
	s.Vars = a
}

func firstLower(s string) string {
	if len(s) == 0 {
		return s
	}

	return strings.ToLower(s[:1]) + s[1:]
}

type templateVar struct {
	Stores  []*StoreTemplate
	Imports []string
	Prefix  string
}

func SchemaToStore(prefix string, stores []*StoreTemplate, imports []string, save bool) error {
	patternMapping := make(map[string]bool)
	nameMapping := make(map[string]bool)
	for _, s := range stores {
		vars := []string{}
		kt := s.Key

		if n, ok := nameMapping[s.Name]; ok {
			fmt.Println("find duplicate name", n)
			return errors.New("find duplicate name")
		}
		nameMapping[s.Name] = true

		pattern := varRegex.ReplaceAllString(kt, "{}")
		if _, ok := patternMapping[pattern]; ok {
			fmt.Println("find duplicate pattern", pattern)
			return errors.New("find duplicate pattern")
		}
		patternMapping[pattern] = true

		matches := varRegex.FindAllStringSubmatch(kt, -1)
		for _, v := range matches {
			vars = append(vars, v[1])
		}
		s.SetVars(vars)
	}

	funcMap := template.FuncMap{
		"FirstLower": firstLower,
	}
	tmpl, err := template.New("store.tmpl").Funcs(funcMap).ParseFS(templateDir, "template/store.tmpl")
	if err != nil {
		fmt.Println(err)
		return err
	}

	b := &bytes.Buffer{}

	err = tmpl.Execute(b, templateVar{
		Stores:  stores,
		Imports: imports,
		Prefix:  prefix,
	})

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var buf []byte
	if buf, err = format.Source(b.Bytes()); err != nil {
		fmt.Println("formating:", err)
		return err
	}

	if !save {
		fmt.Println(string(buf))
		return nil
	}

	if err = ioutil.WriteFile("cacheme/store.go", buf, 0644); err != nil { //nolint
		fmt.Println("writing go file:", err)
		return err
	}
	return nil
}

func InvalidAll(ctx context.Context, group string, client RedisClient) error {
	iter := client.SScan(ctx, group, 0, "", 200).Iterator()
	invalids := []string{}
	for iter.Next(ctx) {
		invalids = append(invalids, iter.Val())
		if len(invalids) == 600 {
			err := client.Unlink(ctx, invalids...).Err()
			if err != nil {
				fmt.Println(err)
			}
			invalids = []string{}
		}
	}

	if len(invalids) > 0 {
		err := client.Unlink(ctx, invalids...).Err()
		if err != nil {
			return err
		}
		err = client.Unlink(ctx, group).Err()
		return err
	}
	return nil
}

func InvalidAllCluster(ctx context.Context, group string, client RedisClient) error {

	clusterClient := client.(*redis.ClusterClient)

	iter := clusterClient.SScan(ctx, group, 0, "", 200).Iterator()
	invalids := make(map[string][]string)
	counter := 0
	for iter.Next(ctx) {

		key := iter.Val()
		node, err := clusterClient.MasterForKey(ctx, key)
		if err != nil {
			return err
		}

		addr := node.Options().Addr

		if v, ok := invalids[addr]; ok {
			v = append(v, key)
			invalids[addr] = v

		} else {
			invalids[addr] = []string{key}
		}
		counter++

		if counter == 600 {

			for _, v := range invalids {

				err := clusterClient.Unlink(ctx, v...).Err()
				if err != nil {
					fmt.Println(err)
				}
			}
			invalids = make(map[string][]string)
			counter = 0
		}
	}

	if counter > 0 {
		for _, v := range invalids {

			err := clusterClient.Unlink(ctx, v...).Err()
			if err != nil {
				fmt.Println(err)
			}
		}
		err := clusterClient.Unlink(ctx, group).Err()
		return err
	}
	return nil
}

func Marshal(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

func Unmarshal(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}