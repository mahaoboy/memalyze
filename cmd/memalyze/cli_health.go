package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// runHealth 是容器健康检查的入口：distroless 镜像里没有 curl/wget，
// 只能靠二进制自己发一个 GET（Dockerfile / compose 的 healthcheck 用它）。
func runHealth(args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	url := fs.String("url", "http://127.0.0.1:8080/healthz", "health endpoint")
	timeout := fs.Duration("timeout", 3*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	client := &http.Client{Timeout: *timeout}
	resp, err := client.Get(*url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: health: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "memalyze: health: %s returned %d\n", *url, resp.StatusCode)
		return 1
	}
	if strings.Contains(*url, "/healthz") {
		fmt.Println("ok")
	}
	return 0
}
