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
	"reflect"
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

type StoreSchema struct {
	Name         string
	Key          string
	To           interface{}
	Version      interface{}
	Vars         []string
	TTL          time.Duration
	Singleflight bool
}

type storeInfo struct {
	StoreSchema
	Type        string
	VersionInfo *versionInfo
}

func (s *StoreSchema) SetVars(a []string) {
	s.Vars = a
}

func firstLower(s string) string {
	if len(s) == 0 {
		return s
	}

	return strings.ToLower(s[:1]) + s[1:]
}

type templateVar struct {
	Stores  []storeInfo
	Imports []string
	Prefix  string
}

func pkgPath(t reflect.Type) string {
	pkg := t.PkgPath()
	if pkg != "" {
		return pkg
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Ptr:
		return pkgPath(t.Elem())

	case reflect.Map:
		return pkgPath(t.Key()) + "|" + pkgPath(t.Elem())
	}
	return pkg
}

type versionInfo struct {
	typ string
}

func (v *versionInfo) IsFunc() bool {
	return v.typ == "func"
}

func (v *versionInfo) IsString() bool {
	return v.typ == "str"
}

func (v *versionInfo) IsInt() bool {
	return v.typ == "int"
}

func getVersionInfo(i interface{}) (*versionInfo, error) {
	typ := reflect.TypeOf(i)
	if typ.Kind() == reflect.String {
		return &versionInfo{typ: "str"}, nil
	}
	if typ.Kind() == reflect.Int {
		return &versionInfo{typ: "int"}, nil
	}
	if typ.Kind() != reflect.Func {
		return nil, errors.New("version type not supported")
	}
	if typ.NumIn() != 0 || typ.NumOut() != 1 || typ.Out(0).Kind() != reflect.String {
		return nil, errors.New("version function not valid")
	}
	return &versionInfo{typ: "func"}, nil
}

func SchemaToStore(schemaPath string, prefix string, stores []*StoreSchema, save bool) error {
	patternMapping := make(map[string]bool)
	nameMapping := make(map[string]bool)
	var info []storeInfo
	importMap := make(map[string]string)

	for _, s := range stores {
		vars := []string{}
		kt := s.Key

		version, err := getVersionInfo(s.Version)
		if err != nil {
			return err
		}
		if version.IsInt() {
			importMap["strconv"] = ""
		}

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
		tmp := strings.Split(s.Name, "")
		s.Name = strings.ToUpper(tmp[0]) + strings.Join(tmp[1:], "")

		t := reflect.TypeOf(s.To)
		path := pkgPath(t)
		info = append(info, storeInfo{
			StoreSchema: *s,
			Type:        t.String(),
			VersionInfo: version,
		})
		all := strings.Split(path, "|")
		for _, p := range all {
			if p != "" {
				importMap[p] = ""
			}
		}
	}

	imports := []string{schemaPath}
	for k := range importMap {
		imports = append(imports, k)
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
		Stores:  info,
		Prefix:  prefix,
		Imports: imports,
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
