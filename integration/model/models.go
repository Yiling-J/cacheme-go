package model

var BarVersion = 6

type Bar struct {
	Name string
}

type Foo struct {
	Name string
	Bar  Bar
	BarP *Bar
}
