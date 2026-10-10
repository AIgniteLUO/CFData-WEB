package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	upstreamHTTPClient = &http.Client{Timeout: 20 * time.Second}

	// directUpstreamHTTPClient 与 upstreamHTTPClient 同源，但显式清空了 Proxy。
	//
	// 它专供「网络身份探测」使用 —— 地区判定与 ISP 判定。这两个探测的职责是代表测速
	// 那条路径说话，而测速与扫描用的是自建的 &http.Transport{} 字面量，Proxy 为空且
	// 直接拨 IP，从不受 HTTP_PROXY 影响。探测一旦走了代理，就会拿代理出口的国家去判定
	// 一条直连路径，报出用户根本没有配置的代理警告；ISP 判定同样会被带偏，让「自动选择
	// 测速源」依据代理出口的运营商做决定（国内用户因此永远选不到「移动专属」）。
	//
	// 会踩到这个坑的机制：http.DefaultTransport 自带 Proxy: ProxyFromEnvironment，
	// Clone() 会把它一并复制过来。而 Docker 会读取 ~/.docker/config.json 的 proxies 段，
	// 自动把 HTTP_PROXY/HTTPS_PROXY 注入容器 —— 用户 compose 里一个字都不写也会中招。
	directUpstreamHTTPClient = &http.Client{Timeout: 20 * time.Second}
)

// locationsURL 是数据中心位置库的下载地址。
var locationsURL = "https://www.baipiao.eu.org/cloudflare/locations"

// newUpstreamTransport 构造与 http.DefaultTransport 同配置的 transport。
// 它保留自带的 ProxyFromEnvironment，调用方若要求直连须自行把 Proxy 置为 nil。
func newUpstreamTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialContext
	transport.TLSClientConfig = tlsConfigWithRootCAs("")
	return transport
}

func configureHTTPClients() {
	initCustomResolver()

	// 保留代理支持：GitHub 上传、edgetunnel 读写、更新检查这些场景，国内用户往往确实需要挂代理。
	upstreamHTTPClient.Transport = wrapDebugTransport("upstream", newUpstreamTransport())

	direct := newUpstreamTransport()
	direct.Proxy = nil
	directUpstreamHTTPClient.Transport = wrapDebugTransport("upstream-direct", direct)

	if description := effectiveProxyDescription(upstreamHTTPClient, ispProbeURL); description != "" {
		fmt.Printf("出网代理: %s\n", description)
		fmt.Println("说明: GitHub 上传等请求会经此代理；地区与 ISP 探测已强制直连，与测速路径一致")
	}
}

// effectiveProxyDescription 返回该客户端会为这个目标地址使用的代理，直连时返回空串。
// 问的是 transport 本身而非环境变量，因此拿到的是真实生效值（含 NO_PROXY 的排除判断）。
func effectiveProxyDescription(client *http.Client, targetURL string) string {
	transport, ok := unwrapTransport(client.Transport)
	if !ok || transport.Proxy == nil {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return ""
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil || proxyURL == nil {
		return ""
	}
	return proxyURL.String()
}

// unwrapTransport 剥掉调试包装，取出真正的 *http.Transport。
func unwrapTransport(rt http.RoundTripper) (*http.Transport, bool) {
	if wrapped, ok := rt.(debugRoundTripper); ok {
		rt = wrapped.next
	}
	transport, ok := rt.(*http.Transport)
	return transport, ok
}

func initLocations() {
	filename := "locations.json"
	url := locationsURL
	var locations []location
	var body []byte
	var err error

	if _, err = os.Stat(filename); os.IsNotExist(err) {
		sendLog("本地 locations.json 不存在，正在从服务器下载...")
		content, err := getURLContent(url)
		if err != nil {
			sendLog("获取位置信息失败: " + err.Error())
			sendLog("⚠️ locationMap 未就绪，后续扫描结果的 城市/地区 列将为空；可删除本地缓存（Web 端右上角设置）或检查网络后重启")
			return
		}
		body = []byte(content)
		if err := saveToFile(filename, content); err != nil {
			sendLog("保存位置信息失败: " + err.Error())
		}
	} else {
		sendLog(fmt.Sprintf("读取本地 %s 文件...", filename))
		body, err = os.ReadFile(filename)
		if err != nil {
			sendLog("读取位置信息失败: " + err.Error())
			sendLog("⚠️ locationMap 未就绪，后续扫描结果的 城市/地区 列将为空")
			return
		}
	}

	if err := json.Unmarshal(body, &locations); err != nil {
		sendLog("解析位置信息失败: " + err.Error())
		sendLog("⚠️ locationMap 未就绪（locations.json 可能损坏），建议删除本地缓存后重启")
		return
	}

	locationMap = make(map[string]location)
	for _, loc := range locations {
		locationMap[loc.Iata] = loc
	}
	fmt.Printf("已加载 %d 个数据中心位置信息\n", len(locationMap))
}

func sendLog(msg string) {
	recordDebugNotice("terminal_log", msg)
	fmt.Println(msg)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}
var utf8BOMString = string(utf8BOM)

func writeUTF8BOM(f *os.File) error {
	_, err := f.Write(utf8BOM)
	return err
}

func resetAllConfigFiles(session *appSession) {
	configResetMutex.Lock()
	defer configResetMutex.Unlock()

	result := resetConfigResult{
		Success:  true,
		Deleted:  []string{},
		Missing:  []string{},
		Failed:   []string{},
		Reminder: "删掉之后，下次重新运行程序时会自动重新下载。",
	}

	session.taskMutex.Lock()
	hasRunningTasks := session.isTaskRunning
	session.taskMutex.Unlock()
	if !hasRunningTasks {
		hasRunningTasks = anyTaskRunning()
	}
	if hasRunningTasks {
		result.Success = false
		result.Failed = []string{"当前还有任务在运行（包含其他连接）"}
		session.sendWSMessage("reset_config_result", result)
		session.sendWSMessage("log", "现在还有任务在跑，先停掉再清理本地缓存文件吧")
		return
	}

	resetASNReader()
	files := []string{"ips-v6.txt", "ips-v4.txt", "locations.json", asnDBFile}
	deleted := []string{}
	missing := []string{}
	failed := []string{}

	for _, filename := range files {
		if err := os.Remove(filename); err == nil {
			deleted = append(deleted, filename)
			continue
		} else if os.IsNotExist(err) {
			missing = append(missing, filename)
		} else {
			failed = append(failed, fmt.Sprintf("%s (%v)", filename, err))
		}
	}

	if len(failed) > 0 {
		result.Success = false
	}
	result.Deleted = deleted
	result.Missing = missing
	result.Failed = failed

	session.sendWSMessage("reset_config_result", result)
	if len(failed) > 0 {
		session.sendWSMessage("log", fmt.Sprintf("本地缓存清理失败: %v", failed))
	} else {
		session.sendWSMessage("log", fmt.Sprintf("本地缓存已清理，已删除: %v，原本不存在: %v", deleted, missing))
	}
	session.sendWSMessage("log", "提示：删掉之后，下次重新运行程序时会自动重新下载。")
}

func getIPListContent(filename, apiURL string) (string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		content, err := getURLContent(apiURL)
		if err != nil {
			return "", err
		}
		if err := saveToFile(filename, content); err != nil {
			fmt.Println("保存 IP 列表缓存失败:", err)
		}
		return content, nil
	}
	return getFileContent(filename)
}

func getURLContent(targetURL string) (string, error) {
	return getURLContentWithContext(context.Background(), targetURL)
}

func getURLContentWithContext(ctx context.Context, targetURL string) (string, error) {
	data, err := getURLBytesWithContext(ctx, targetURL)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getURLBytes(targetURL string) ([]byte, error) {
	return getURLBytesWithContext(context.Background(), targetURL)
}

func getURLBytesWithContext(ctx context.Context, targetURL string) ([]byte, error) {
	return fetchURLBytes(ctx, upstreamHTTPClient, targetURL)
}

// getURLBytesDirectWithContext 走直连客户端，专供网络身份探测使用。
// 理由见 directUpstreamHTTPClient 的说明：探测必须与测速走同一条路径。
func getURLBytesDirectWithContext(ctx context.Context, targetURL string) ([]byte, error) {
	return fetchURLBytes(ctx, directUpstreamHTTPClient, targetURL)
}

func fetchURLBytes(ctx context.Context, client *http.Client, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("请求失败: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func getFileContent(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func saveToFile(filename, content string) error {
	return atomicWriteFile(filename, []byte(content), 0644)
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func parseCSVFile(filename string) ([]string, [][]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}

	headers := rows[0]
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], utf8BOMString)
	}
	return headers, rows[1:], nil
}
