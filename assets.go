package main

import "embed"

// assets is populated by the frontend build before Wails packages the app.
//
//go:embed all:frontend/dist
var assets embed.FS
