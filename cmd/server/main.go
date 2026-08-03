package main

import "go-data/internal/app"
import "go-data/internal/config"

func main() {
	cfg := config.Load()
	app.Build(cfg)
}
