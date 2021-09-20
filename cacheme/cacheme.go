package cacheme

import (
	"bytes"
	"context"
	"embed"
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
)

//go:embed template/*
var templateDir embed.FS

var varRegex = regexp.MustCompile(`{{.([a-zA-Z0-9]+)}}`)

type CacheStore interface {
	Initialized() bool
	Tag() string
	AddMemoLock() error
	SetClient(*redis.Client)
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

func NewPipeline(client *redis.Client) *CachePipeline {
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

func SchemaToStore(prefix string, stores []*StoreTemplate, imports []string) {
	for _, s := range stores {
		vars := []string{}
		kt := s.Key
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
		os.Exit(1)
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
		fmt.Println("formatting output:", err)
		os.Exit(1)
	}

	if err = ioutil.WriteFile("cacheme/store.go", buf, 0644); err != nil { //nolint
		fmt.Println("writing go file:", err)
		os.Exit(1)
	}
}
