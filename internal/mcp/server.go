// Package mcp 组装 memalyze 的 MCP server 与工具。
//
// 设计约束：
//   - 只做「中间件」：不抢用户的 harness，通过 MCP 被任何客户端接入。
//   - 工具名与参数保持稳定，后续 S12 的 memory_* 三件套在此扩展。
package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Serve 在 stdio 传输上启动 MCP server，阻塞直到客户端断开。
func Serve(ctx context.Context, version string) error {
	return NewServer(version).Run(ctx, &mcpsdk.StdioTransport{})
}

// NewServer 构建 server 实例并注册全部工具。
func NewServer(version string) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "memalyze",
		Version: version,
	}, nil)

	registerEcho(server)
	return server
}

// echoArgs 是 echo 工具入参。jsonschema tag 由 SDK 自动转成 JSON Schema。
type echoArgs struct {
	Text string `json:"text" jsonschema:"the text to echo back"`
}

// registerEcho 注册 S6 的 hello-world 工具：echo(text) -> text。
// 它的唯一职责是证明 MCP 链路通（工具可见、可调用），不做任何业务逻辑。
func registerEcho(server *mcpsdk.Server) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "echo",
		Description: "Echo the given text back unchanged. Used to verify the MCP wiring.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args echoArgs) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: args.Text}},
		}, nil, nil
	})
}
