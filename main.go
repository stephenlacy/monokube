package main

import (
	"github.com/fatih/color"
	"github.com/stevelacy/monokube/pkg/monokube"
)

var Version string

func main() {
	color.Cyan("monokube version: %s\n", Version)
	monokube.Init()
}
