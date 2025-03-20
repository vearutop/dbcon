// Package main is an example CLI app.
package main

import (
	"log"

	"github.com/vearutop/dbcon/app"
)

func main() {
	if err := app.Main(); err != nil {
		log.Fatal(err)
	}
}
