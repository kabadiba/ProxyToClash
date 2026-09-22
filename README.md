# ProxyToClash

把代理IP服务商给你的「提取API」，自动转成 Clash 能直接用的代理源，
并帮你**自动切换出口 IP**，Windows / Linux / Android 都能用。

---

## 它解决什么问题

代理IP服务商卖的一般是「动态短效 IP」：它的提取API每次吐给你一批 `ip:port`，
这些 IP **有时效、过期作废**，需要反复去拉。而 Clash 自己不会去拉服务商的
提取API，只认固定格式的代理配置。

这个程序就是中间的「翻译 + 换班员」：

```
你的提取API  ──►  ProxyToClash 定时/按需拉取、转成配置 ──►  你的 Clash 自动拉取
  (一堆ip:port)      (帮你处理时效、格式、失效)                (自动挑可用最快的出口IP)
```

你唯一要做的：**填一次你的提取API地址**，之后 IP 的提取、转换、换新、失效剔除，全部自动。

---

## 需要准备

- **一台常开的机器**：Windows 或 Linux 都行（放代理转换器的地方）。
- 代理服务商的**「动态短效提取API」地址**（登录服务商后台就能拿到，选返回 `ip:port` 换行文本的那种格式最通用）。
- 三端里任一端装上支持 `proxy-providers` 的 Clash 客户端（见下）。

---

## 快速开始（三步）

### 第 1 步：拿到程序
用现成编译好的（Windows）：

```
build\ProxyToClash.exe
```

（Linux 见文末「从源码构建」；或直接用 `scripts/build_all.sh` 产出。）

### 第 2 步：填你的提取API
双击运行一次——它会**自动生成 `config.json`**，然后退出。用记事本打开 `config.json`，
把 `api_url` 改成你的真实提取API地址，其余默认即可：

```json
{
  "api_url": "http://你看的提取API完整地址",
  "proxy_type": "http",
  "refresh_interval_sec": 300,
  "on_demand": true,
  "on_demand_min_sec": 30,
  "http_port": 8080
}
```

字段说明：

| 字段 | 意思 | 默认 |
|---|---|---|
| `api_url` | 你的提取API地址（**必填**） | 空 |
| `proxy_type` | 协议 `http` 或 `socks5`，跟你套餐一致 | `http` |
| `refresh_interval_sec` | 定时刷新间隔(秒)；**设 `0` 关掉定时**，只靠按需刷新 | `300` |
| `on_demand` | Clash 每次拉取时，现场提取最新IP再返回（开） | `true` |
| `on_demand_min_sec` | 两次现场提取的最小间隔(秒)，防止被频繁调用烧额度 | `30` |
| `bypass_proxy` | 提取请求是否**绕过代理直连**（转换器和 Clash 同机、Clash 会接管流量时开） | `false` |
| `proxy_username`/`proxy_password` | 你的IP需要账号密码时才填 | 空 |
| `http_port` | 推送IP文件给 Clash 的端口 | `8080` |

### 第 3 步：跑起来，并让 Clash 用它
再次运行 `ProxyToClash`（或双击）。浏览器打开 `http://127.0.0.1:8080/ips.yaml`
应该能看到一堆代理节点，说明转换成功。

然后按下面的「接入 Clash」把你的 Clash 客户端指到这个地址即可。

---

## 刷新策略：定时 vs 按需实时

IP 有时效，Clash 拉到的必须是新鲜的，否则就得刚拉就过期。程序内置两种刷新，默认都开：

- **定时兜底**：每 `refresh_interval_sec` 拉一批，保证文件永远新鲜。
- **按需实时**：Clash 每次来拉 `ips.yaml`，程序就当场去提取API拉一批**最新**的再返回；
  这就是为了避免「Clash 拉到的马上要过期的旧 IP」——从机制上消除错位。

> **注意**：开了按需实时后，你的提取API大概会**被调得很频繁**（每次都调用、受
> `on_demand_min_sec` 兜底）。IP 是按量计费的，建议在 Clash 端把拉取间隔设成 60s
> 左右（见下），别设太短。实时提取失败时程序会保留上一次成功结果，不会让你断网。

想只用其中一种：
- 只要按需实时：`refresh_interval_sec` 设 `0`
- 只要定时：`on_demand` 设 `false`

---

## 接入 Clash（Windows / Linux / Android 三端任选）

打开 `templates/clash-config.yaml`，只需把**一个 URL** 改成你的实际情况，然后整份粘贴进你的 Clash 客户端：

- 转换器就在本机（Windows/Linux）：`http://127.0.0.1:8080/ips.yaml`
- Android（手机连的是那台机器所在的 WiFi）：`http://那台机器的局域网IP:8080/ips.yaml`
  （查机器 IP：Windows 用 `ipconfig`，Linux 用 `ip addr`）

| 端 | 推荐客户端 | 开机自动运行 |
|---|---|---|
| Windows | Clash Verge / mihomo | 转换器用 `deploy/windows/计划任务.txt` 配一步自启 |
| Linux | mihomo core | 转换器用 `deploy/linux/proxytoclash.service` 配 systemd |
| Android | **Clash Meta for Android (CMFA)** | 手机端自带自启动/自动更新开关 |

> Android 只需要导入配置，**不用跑任何程序**。即使一时连不上那台机器，Clash 也还留着上次的IP可以用。

---

## 三种自动切换策略（想用哪种选哪种）

都在那份 Clash 配置里配好了，在客户端界面点一下「策略入口」下拉即可切换：

| 策略 | 做什么 | 适合 |
|---|---|---|
| 自动测速选优 | 每隔一段时间测各IP延迟，自动用最快的，失效的自动剔除 | 要「稳定能用的出口」 |
| 轮询换IP | 每次新连接换一个IP | 要「每访问换不同IP」防止被识别 |
| 手动/定时强制切 | 用命令把出口钉到指定某个IP | 你控制切换的时机 |

手动/定时强制切的命令（Windows/Linux 本机）：
```
curl -X PATCH "http://127.0.0.1:9090/proxies/自动测速选优" -H "Authorization: Bearer 你的token" -d '{"name":"p2"}'
```
把这条命令放进 Windows 计划任务 / Linux cron，就是「定时换 IP」。

---

## 常见问题

- **为什么所有IP都不好用/全被判不可用？** Clash 里测速地址请保持国内能访问的
  `http://www.baidu.com/generate_204`，别换成外网地址，否则国内IP会被误判失效。
- **IP不新鲜/老掉线？** 把 `refresh_interval_sec` 设得比 IP 的时效小一点（留一倍余量）。
- **IP要账号密码？** 在 `config.json` 填 `proxy_username` / `proxy_password` 即可。
- **Android 换了手机流量就连不上？** 只用局域网的话这是正常的，Clash 会沿用缓存的IP；
  想随时能刷新，得给那台机器配公网或内网穿透（本项目默认按局域网用，不强制）。
- **跑起来报「api_url 为空」？** 说明还没填 `config.json` 里的 `api_url`。
- **转换器和 Clash 同装一台机器，担心代理故障会拖垮提取、陷入死循环？** 拆成两半看：
  Clash 从本机 `127.0.0.1` 拉 provider 一般走直连、不受代理影响（这一路总通）；
  真正会被代理绑架的是转换器调**提取API**的公网请求——把 `bypass_proxy` 设 `true` 强制直连，
  并按 `templates/clash-config.yaml` 里的说明把**服务商API域名**放行为 `DIRECT`，
  就能在代理故障时照常拉到新IP、正常自愈。

---

## 从源码构建（可选，一般用现成二进制即可）

需要装了 Go（≥1.21）的机器，一条命令出单个文件，无任何依赖：

```bash
# Windows 本机
go build -trimpath -ldflags "-s -w" -o build/ProxyToClash.exe .
# Linux 服务器或旧版机能用的跨平台编译
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o build/ProxyToClash_linux .
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o build/ProxyToClash_linux_arm64 .
```

一键脚本：Windows 跑 `scripts/build_win.bat`（单平台）
或 `scripts/build_all.bat`（一次出全平台）；Linux 跑 `sh scripts/build_all.sh`。
产物都在 `build/`（该目录已被 git 忽略，不会误提交）。

---
目录说明（给想深入看的人）：`main.go` 程序源码，`templates/clash-config.yaml` 三端共用 Clash 配置，
`config.json.example` 配置样例，`deploy/` 开机自启教程，`scripts/` 构建脚本。