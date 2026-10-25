# 添丁记（FamilyLedger）

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

家庭共用的人情往来 + 宝宝开销记账。Go 单二进制 + SQLite 单文件，前端已内嵌，无外部依赖，数据全在你自己的机器上。

> **English**: A self-hosted family ledger for Chinese "人情往来" (gift/reciprocity bookkeeping) and baby expenses.
> Single Go binary + single SQLite file, frontend embedded, zero external dependencies.

## 功能

### 洞察（普通记账软件没有的部分）

- **消耗品补货预警**：奶粉/尿不湿等记录数量后，按相邻两次购买反推日均消耗，算出「还剩几天用完」及预计见底日期
- **人情紧迫度**：待回礼按「最后一次收到对方礼金至今」排序，拖得越久越靠前（昨天刚回过礼 ≠ 两年前收了没还）
- **久未往来提醒**：超过两年没走动的关系主动提示，人情是走动出来的
- **养育成本预测**：按近三个月花销推算一年开销，设了宝宝生日后还能推算到 3 岁的累计花费
- **成长时间线**：花销、收送礼、里程碑揉在同一条轴上，账本同时也是日记本

### 基本盘

- **人情账本**：收/送礼、按人差额、待回礼清单、按事由统计
- **添丁记**：12 类分类开销、数量与单位、环形图占比、近半年走势

### 记录管理

- **编辑任意记录**：金额、方向、日期、事由、备注、回礼状态都能改，不用删了重记
- **批量录礼单**：办酒席时粘一长串「名字 金额」一次录几十笔，实时预览条数与合计
- **筛选**：按关系（亲戚/同事/同学…）、方向、金额区间、关键词（姓名/事由/备注）
- **区间切换**：本周 / 本月 / 本季 / 本年 / 全部，每项都带**环比对比**（与上一周期相比涨了多少）
- **日历热力图**：近 5 周按天着色，一眼看出哪天花得多
- **往来 Top**：本期往来金额最高的人
- **回礼标记**：可把某笔标为「无需回礼」（如长辈压岁钱），不再进待回礼清单
- **完整备份 / 恢复**：导出 JSON 快照，换机器时导入即可（同名人和事由自动合并）
- **成长里程碑**：15 种预设节点（出生/满月/翻身/长牙…），每条自动标注宝宝当时多大
- **多成员**：全家一个口令，各选身份，每笔记住谁录的
- **PWA**：手机「添加到主屏幕」全屏运行，安卓/iPhone 均支持，底部导航
- **离线录入**：断网写入本地队列，恢复后自动同步
- **CSV 导出**：两本账分别导出，UTF-8 BOM，Excel 直接打开不乱码
- **每日自动备份**：凌晨 3 点 `VACUUM INTO` 一致性快照（不停服），保留 30 份

## 快速开始

### 从源码编译

需要 Go 1.27 或更高版本（见 [`go.mod`](./go.mod)）。项目用的是纯 Go 的 SQLite 驱动（modernc.org/sqlite），
**不需要 CGO，交叉编译很省事**：

```bash
# 本机编译
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o familyledger .

# 交叉编译
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/familyledger-linux-amd64 .
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/familyledger-linux-arm64 .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/familyledger.exe .
```

编译产物默认输出到 `dist/`，该目录已在 `.gitignore` 中不入库。

### 运行

```bash
FL_DB=./test.db FL_SHARED_TOKEN=123456 ./familyledger   # Linux
# Windows: set FL_DB=test.db && set FL_SHARED_TOKEN=123456 && familyledger.exe
```

打开 <http://127.0.0.1:8080> ，口令 123456。

> ⚠️ `123456` 只是默认值，**部署到公网前务必换成自己的口令**。

### Docker

用 [`Dockerfile`](./Dockerfile) 直接构建镜像，或参考 `deploy/` 下的 compose 文件。

## 环境变量

| 变量                | 默认                      | 说明                  |
| ----------------- | ----------------------- | ------------------- |
| `FL_DB`           | `/data/familyledger.db` | SQLite 路径（所在目录自动创建） |
| `FL_ADDR`         | `:8080`                 | 监听地址                |
| `FL_SHARED_TOKEN` | `123456`                | **家庭口令，务必修改**       |
| `FL_BACKUP_DIR`   | 空                       | 设置后启用每日凌晨 3 点备份     |

## 飞牛 NAS 部署（全程图形界面）

1. NAS 上建目录 `/vol1/docker/familyledger/`（里面建 `data`、`backup` 两个空文件夹）
2. 打开 **Docker → Compose → 新增项目**
   - 项目名称：`familyledger`
   - 来源选「创建 docker-compose.yml」，粘贴 `deploy/docker-compose.yml` 的内容
   - **先把里面的 `image` 换成你的实际仓库地址、`FL_SHARED_TOKEN` 换成 6 位口令**
3. 勾选「创建项目后立即启动」→ 完成
4. 局域网浏览器打开 `http://<NAS IP>:8080` 验证能登录、能记账

不想打镜像的话用 `deploy/docker-compose.免构建.yml`：把编译好的二进制挂进 alpine 容器直接跑，
步骤见 [`部署说明.md`](./部署说明.md)。

## 数据迁移（换机器/上云 = 拷两个文件）

```
backup/fl-2026-09-25.db   # 任意一份快照即完整数据
familyledger-linux-amd64  # 对应架构的二进制
```

云服务器上：`./familyledger` 直接跑（无需 Docker），配 systemd 常驻即可。

## 目录结构

```
.
├── main.go          入口、路由、静态资源内嵌
├── api.go           认证与会话
├── store.go         SQLite 数据层（建表、迁移、查询）
├── insight.go       洞察引擎（补货预警 / 人情紧迫度 / 成本预测）
├── features.go      业务功能（礼单、开销、里程碑…）
├── health.go        健康检查与每日备份
├── web/             前端（内嵌进二进制，改完需重新编译）
├── deploy/          Docker Compose 与部署脚本
└── Dockerfile
```

## 技术要点

- 金额全程**整数分**存储，前端任意格式输入（`1,200` / `600.5` / `¥800`）后端统一解析
- SQLite WAL 模式 + busy_timeout，两人同时记账安全
- token 双写 localStorage + HttpOnly Cookie，iOS 清掉 localStorage 后仍可凭 Cookie 续命，最多重登一次
- Service Worker：静态资源 cache-first，**API 一律 network-only**，数据永远最新

## 协议

[MIT](./LICENSE) — 随便用、随便改，保留署名即可。
