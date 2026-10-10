package main

import (
	"net/http"
	"net/url"
	"testing"
)

// proxyFor 询问 transport 实际会给这个请求用什么代理；nil 表示直连。
func proxyFor(t *testing.T, client *http.Client, targetURL string) *url.URL {
	t.Helper()
	transport, ok := unwrapTransport(client.Transport)
	if !ok {
		t.Fatalf("transport 不是 *http.Transport，而是 %T", client.Transport)
	}
	if transport.Proxy == nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("解析代理失败: %v", err)
	}
	return proxyURL
}

// 网络身份探测（地区判定、ISP 判定）必须与测速走同一条直连路径。
//
// 测速与扫描用的是自建的 &http.Transport{} 字面量，Proxy 为空、且直接拨 IP，
// 所以它们从不受 HTTP_PROXY 影响。而探测走的是 clone 自 http.DefaultTransport 的
// 通用客户端，DefaultTransport 自带 Proxy: ProxyFromEnvironment，Clone 会一并复制，
// 于是探测会被代理劫持 —— 拿代理出口的国家去判定一条直连路径，报出用户根本没设的代理警告。
// Docker 会从 ~/.docker/config.json 的 proxies 段自动把 HTTP_PROXY 注入容器，正是这个坑的常见来源。
func TestDirectClientBypassesProxy(t *testing.T) {
	configureHTTPClients()

	directTransport, ok := unwrapTransport(directUpstreamHTTPClient.Transport)
	if !ok {
		t.Fatalf("直连客户端 transport 类型异常: %T", directUpstreamHTTPClient.Transport)
	}
	if directTransport.Proxy != nil {
		t.Error("直连客户端不得设置 Proxy：一旦设置，HTTP_PROXY/HTTPS_PROXY 会再次劫持网络身份探测")
	}

	upstreamTransport, ok := unwrapTransport(upstreamHTTPClient.Transport)
	if !ok {
		t.Fatalf("通用上游客户端 transport 类型异常: %T", upstreamHTTPClient.Transport)
	}
	if upstreamTransport.Proxy == nil {
		t.Error("通用上游客户端应保留 ProxyFromEnvironment：GitHub 上传、edgetunnel 读写等场景国内用户确实需要代理")
	}
}

// 直连客户端在任何目标上都不该解析出代理，这一条与运行环境的 proxy 变量无关，必须恒定成立。
func TestDirectClientResolvesNoProxyForAnyTarget(t *testing.T) {
	configureHTTPClients()

	for _, target := range []string{
		cloudflareTraceURL,
		ispProbeURL,
		"https://api.github.com/repos/PoemMisty/CFData-WEB/releases/latest",
		locationsURL,
	} {
		if got := proxyFor(t, directUpstreamHTTPClient, target); got != nil {
			t.Errorf("目标 %s 不应解析出代理，实际得到 %s", target, got)
		}
	}
}

// 用户在启动时应当能看见「代理确实生效了」这件事。
// 之前这个故障难排查，根子就在于全程没有任何一处提到代理的存在。
func TestEffectiveProxyDescription(t *testing.T) {
	configureHTTPClients()

	if got := effectiveProxyDescription(directUpstreamHTTPClient, cloudflareTraceURL); got != "" {
		t.Errorf("直连客户端不应报告代理，实际 %q", got)
	}

	// 无代理函数时不能谎报
	if got := effectiveProxyDescription(&http.Client{}, cloudflareTraceURL); got != "" {
		t.Errorf("未配置 transport 的客户端不应报告代理，实际 %q", got)
	}
}
