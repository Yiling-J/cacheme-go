package model

type Bar struct {
	Name string
}

type Foo struct {
	Name string
	Bar  Bar
	BarP *Bar
}
