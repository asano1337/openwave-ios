//go:build !windows

package main

import "net/http"

func runApp(address string, srv *http.Server) { openWindow(address); select {} }
