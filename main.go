// ProxyToClash 转换器（Go 原生单文件，无第三方依赖）
//
// 功能：把代理商"动态短效IP提取API"返回的 ip:port 列表转成 mihomo proxy-provider
//   认得的 proxies 片段写入 ips.yaml，并通过一个极简 HTTP 服务在局域网暴露，
//   供 Windows/Linux/Android 三端 Clash 内核拉取 + 自动切换出口IP。
//
// 两种刷新策略（config.json 配置）：
//   定时(timer)：后台每 refresh_interval_sec 拉一次（兜底，保证文件新鲜）。
//   按需(on_demand)：Clash 拉取 ips.yaml 时，若距上次生成 >= on_demand_min_sec，
//      则现场调提取API生成最新内容再返回；否则返回最近一次结果。
//   默认两者都开（hybrid）：定时兜底 + Clash 每次拉取尽量拿到最新IP，
//      彻底避免"Clash 拉到即将被替换的旧IP"。
//
// 配置：读同目录 config.json（首次运行自动生成样例，编辑 api_url 后重跑）
// 构建：
//   Windows:  go build -trimpath -ldflags "-s -w" -o build/ProxyToClash.exe .
//   Linux:    GOOS=linux  GOARCH=amd64 go build -o build/ProxyToClash_linux .
//   树莓派:   GOOS=linux  GOARCH=arm64 go build -o build/ProxyToClash_linux_arm64 .
//
// 用法：
//   ProxyToClash                前台运行（刷新循环 + HTTP 服务）
//   ProxyToClash -once          只拉取写文件后退出（配合 cron/计划任务）
//   ProxyToClash -config=xx.json 指定配置文件
//   ProxyToClash -port=9000      临时覆盖端口（不写文件）
//   ProxyToClash -interval=120   临时覆盖定时刷新间隔（秒）
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultConfigFile = "config.json"
	defaultOutFile    = "ips.yaml"
)

// Config 对应 config.json 的字段
type Config struct {
	APIURL        string `json:"api_url"`                   // 代理商提取API完整URL
	ProxyType     string `json:"proxy_type"`                // http 或 socks5
	ProxyUsername string `json:"proxy_username"` // 若IP需要账密则填写，缺省为空
	ProxyPassword string `json:"proxy_password"` // 同上
	RefreshSec    int    `json:"refresh_interval_sec"`      // 定时刷新间隔(秒)，0=关闭定时(仅用按需)
	OnDemand      bool   `json:"on_demand"`                 // 是否开启:Clash拉取时按需实时提取
	OnDemandMin   int    `json:"on_demand_min_sec"`         // 按需模式两次实时提取的最小间隔(秒)，限流
	BypassProxy   bool   `json:"bypass_proxy"`              // 提取请求是否绕过代理直连(同机Clash接管时建议开)
	OutFile       string `json:"out_file"`                 // 输出文件
	HTTPHost      string `json:"http_host"`                 // 0.0.0.0 供局域网访问
	HTTPPort      int    `json:"http_port"`                 // HTTP 端口
}

func defaultConfig() Config {
	return Config{
		APIURL:     "",
		ProxyType:  "http",
		RefreshSec: 300,
		OnDemand:   true,
		OnDemandMin: 30,
		OutFile:    defaultOutFile,
		HTTPHost:   "0.0.0.0",
		HTTPPort:   8080,
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if !fileExists(path) {
		writeJSON(path, cfg)
		return cfg, fmt.Errorf("已生成配置模板 %s，请编辑填入 api_url 后重新运行", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	// 补默认值
	d := defaultConfig()
	if cfg.ProxyType == "" {
		cfg.ProxyType = d.ProxyType
	}
	if cfg.OutFile == "" {
		cfg.OutFile = d.OutFile
	}
	if cfg.HTTPHost == "" {
		cfg.HTTPHost = d.HTTPHost
	}
	if cfg.HTTPPort <= 0 {
		cfg.HTTPPort = d.HTTPPort
	}
	// RefreshSec: 缺省用默认值(由 defaultConfig 预填 300)；显式填 0 = 关闭定时刷新
	if cfg.OnDemandMin <= 0 {
		cfg.OnDemandMin = d.OnDemandMin
	}
	if cfg.APIURL == "" {
		return cfg, fmt.Errorf("%s 中 api_url 为空，请填写你的代理商提取API地址", path)
	}
	return cfg, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

var ipLineRE = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}:\d{1,5}$`)

func fetchIPs(apiURL string, bypass bool) ([]string, error) {
	client := http.DefaultClient
	if bypass { // 提取请求强制直连，避免被同机Clash的(可能故障的)代理IP接管
		client = &http.Client{
			Transport: &http.Transport{Proxy: nil}, // nil 关闭代理
			Timeout:   20 * time.Second,
		}
	}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回 HTTP %d", resp.StatusCode)
	}
	seen := make(map[string]bool)
	var lines []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || seen[l] || !ipLineRE.MatchString(l) {
			continue
		}
		seen[l] = true
		lines = append(lines, l)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, errors.New("API 返回为空（套餐耗尽 / 参数错 / 被限流？）")
	}
	return lines, nil
}

func buildProxiesYAML(lines []string, cfg Config) string {
	var b strings.Builder
	b.WriteString("proxies:\n")
	for i, l := range lines {
		idx := strings.LastIndex(l, ":")
		host, portStr := l[:idx], l[idx+1:]
		port, _ := strconv.Atoi(portStr)
		fmt.Fprintf(&b, "  - name: \"p%d\"\n", i)
		fmt.Fprintf(&b, "    type: %s\n", cfg.ProxyType)
		fmt.Fprintf(&b, "    server: %s\n", host)
		fmt.Fprintf(&b, "    port: %d\n", port)
		if cfg.ProxyUsername != "" {
			fmt.Fprintf(&b, "    username: %s\n", cfg.ProxyUsername)
			fmt.Fprintf(&b, "    password: %s\n", cfg.ProxyPassword)
		}
	}
	return b.String()
}

// atomicWrite 先写临时文件再改名，避免 Clash 拉到写了一半的文件
func atomicWrite(path, data string) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func refreshOnce(cfg Config) error {
	lines, err := fetchIPs(cfg.APIURL, cfg.BypassProxy)
	if err != nil {
		return err
	}
	if err := atomicWrite(cfg.OutFile, buildProxiesYAML(lines, cfg)); err != nil {
		return err
	}
	log.Printf("wrote %d proxies -> %s\n", len(lines), cfg.OutFile)
	return nil
}

// state 持有配置与最近一次生成时间；用锁保证并发安全下的限流
type state struct {
	mu      sync.Mutex
	cfg     Config
	lastGen time.Time
}

// refresh 拉取并写文件，成功则更新 lastGen
func (s *state) refresh() {
	if err := refreshOnce(s.cfg); err != nil {
		log.Println("fetch failed, keep last file:", err)
		return
	}
	s.lastGen = time.Now()
}

// currentYAML 返回当前 ips.yaml 内容。
// on_demand 开启时，若距上次生成已超过最小间隔，则现场刷新一次再返回，
// 让 Clash 每次拉取都尽量拿到刚刚提取的最新IP。
func (s *state) currentYAML() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.OnDemand && time.Since(s.lastGen) >= time.Duration(s.cfg.OnDemandMin)*time.Second {
		s.refresh() // 复用锁；失败时会保留上次文件并记录日志
	}
	return os.ReadFile(s.cfg.OutFile)
}

func (s *state) ticker() {
	for {
		time.Sleep(time.Duration(s.cfg.RefreshSec) * time.Second)
		s.mu.Lock()
		s.refresh()
		s.mu.Unlock()
	}
}

func makeHandler(s *state) http.Handler {
	mux := http.NewServeMux()
	target := "/" + s.cfg.OutFile
	mux.HandleFunc(target, func(w http.ResponseWriter, r *http.Request) {
		data, err := s.currentYAML()
		if err != nil {
			http.Error(w, "provider not available", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write(data)
	})
	return mux
}

func main() {
	cfgPath := flag.String("config", defaultConfigFile, "config file path")
	once := flag.Bool("once", false, "fetch once and write file, then exit")
	port := flag.Int("port", 0, "override HTTP port")
	interval := flag.Int("interval", 0, "override timer refresh interval (s)")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalln("run failed:", err)
	}
	if *port > 0 {
		cfg.HTTPPort = *port
	}
	if *interval > 0 {
		cfg.RefreshSec = *interval
	}

	http.DefaultClient.Timeout = 20 * time.Second

	if *once {
		if err := refreshOnce(cfg); err != nil {
			log.Fatalln("fetch failed:", err)
		}
		return
	}

	s := &state{cfg: cfg, lastGen: time.Now()}
	s.mu.Lock()
	s.refresh() // 启动即先取一份，确保文件可被拉取
	s.mu.Unlock()
	if s.cfg.RefreshSec > 0 {
		go s.ticker()
	} else {
		log.Println("timer disabled (refresh_interval_sec=0): refresh via on_demand only")
	}

	addr := net.JoinHostPort(cfg.HTTPHost, strconv.Itoa(cfg.HTTPPort))
	log.Printf("serving http://%s/%s  (timer=%ds, on_demand=%v/min%ds)\n",
		addr, cfg.OutFile, cfg.RefreshSec, cfg.OnDemand, cfg.OnDemandMin)
	if err := http.ListenAndServe(addr, makeHandler(s)); err != nil {
		log.Fatalln("http server failed:", err)
	}
}