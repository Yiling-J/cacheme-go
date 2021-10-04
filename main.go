package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"os/exec"
	"text/template"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

var schemaCode = `
package schema

import (
	"github.com/Yiling-J/cacheme-go/cacheme"
)

var (
        // default prefix for redis keys
	Prefix  = "cacheme"

        // extra imports in generated file
	Imports = []string{}

        // store templates
	Stores = []*cacheme.StoreTemplate{}
)
`

var generateCode = `
package main

import "os"
import cm "github.com/Yiling-J/cacheme-go/cacheme"
import schema "{{.}}/cacheme/schema"


func main() {
    err := cm.SchemaToStore(schema.Prefix, schema.Stores, schema.Imports, true)
    if err != nil {
        os.Exit(1)
    }
}

`

func get(target string) (string, error) {
	cmd := exec.Command("go", "get", target)
	stderr := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	if err := cmd.Run(); err != nil {
		fmt.Println(stdout.String())
		fmt.Println(stderr.String())
		return "", fmt.Errorf("get error: %s", stderr)
	}
	fmt.Println(stdout.String())
	return stdout.String(), nil
}

func run(target string) (string, error) {
	cmd := exec.Command("go", "run", target)
	stderr := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	if err := cmd.Run(); err != nil {
		fmt.Println(stdout.String())
		fmt.Println(stderr.String())
		return "", fmt.Errorf("generate error: %s", stderr)
	}
	fmt.Println(stdout.String())
	return stdout.String(), nil
}

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "init cacheme schema file",
		Run: func(cmd *cobra.Command, args []string) {
			target := "cacheme/schema/schema.go"
			if err := os.MkdirAll("cacheme/schema", os.ModePerm); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			tmpl, err := template.New("init").Parse(schemaCode)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			b := &bytes.Buffer{}
			err = tmpl.Execute(b, nil)

			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			var buf []byte
			if buf, err = format.Source(b.Bytes()); err != nil {
				fmt.Println("formatting output:", err)
				os.Exit(1)
			}
			// nolint: gosec
			if err := ioutil.WriteFile(target, buf, 0644); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
	return cmd
}

func generateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "generate cache.go",
		Run: func(cmd *cobra.Command, args []string) {
			target := "cacheme/.gen/main.go"

			if err := os.MkdirAll("cacheme/.gen", os.ModePerm); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			cfg := &packages.Config{Mode: packages.NeedFiles | packages.NeedSyntax}
			pkgs, err := packages.Load(cfg, ".")
			if err != nil {
				fmt.Println("Can't load package: ", err)
				os.Exit(1)
			}

			pkg := pkgs[0].ID
			fmt.Println(pkgs, pkgs[0].ID, pkgs[0].Name, pkgs[0].Module)

			tmpl, err := template.New("generare").Parse(generateCode)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			b := &bytes.Buffer{}
			err = tmpl.Execute(b, pkg)

			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			var buf []byte
			if buf, err = format.Source(b.Bytes()); err != nil {
				fmt.Println("formatting output:", err)
				os.Exit(1)
			}
			// nolint: gosec
			if err := ioutil.WriteFile(target, buf, 0644); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			defer os.RemoveAll("cacheme/.gen")
			_, err = get("github.com/go-redis/redis/v8")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			_, err = run(target)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
	return cmd
}

func main() {
	cmd := &cobra.Command{Use: "cacheme"}
	cmd.AddCommand(
		initCmd(),
		generateCmd(),
	)
	_ = cmd.Execute()
}
