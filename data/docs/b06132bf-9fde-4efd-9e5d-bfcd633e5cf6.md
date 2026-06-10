# 巡检报告：通用服务器巡检

## 巡检概览

- 环境：test
- 工作流：通用服务器巡检
- 执行 ID：`592ed646-e647-4f54-99b0-c92251f51ebd`
- 开始时间：2026-06-11T02:10:53+08:00
- 结束时间：2026-06-11T02:10:53+08:00（耗时 1s）
- 总体状态：**Success**
- 项数统计：成功 10

## 检查结果

### 1. 系统信息 — ✅ 成功

```sh
uname -a && cat /etc/os-release | head -5
```

```
[info] 执行远程命令: uname -a && cat /etc/os-release | head -5
[info] 命令执行成功
[info] stdout: Linux lgx-vmware-virtual-platform 7.0.0-22-generic #22-Ubuntu SMP PREEMPT_DYNAMIC Mon May 25 15:54:34 UTC 2026 x86_64 GNU/Linux
PRETTY_NAME="Ubuntu 26.04 LTS"
NAME="Ubuntu"
VERSION_ID="26.04"
VERSION="26.04 (Resolute Raccoon)"
VERSION_CODENAME=resolute
```

### 2. 系统负载 — ✅ 成功

```sh
uptime
```

```
[info] 执行远程命令: uptime
[info] 命令执行成功
[info] stdout: 03:10:53 up  5:12,  5 users,  load average: 0.24, 0.28, 0.19
```

### 3. 内存使用 — ✅ 成功

```sh
free -m
```

```
[info] 执行远程命令: free -m
[info] 命令执行成功
[info] stdout: total        used        free      shared  buff/cache   available
内存：         27358        3104       20472          13        4189       24254
交换：             0           0           0
```

### 4. 磁盘使用 — ✅ 成功

```sh
df -h
```

```
[info] 执行远程命令: df -h
[info] 命令执行成功
[info] stdout: 文件系统        容量  已用  可用 已用% 挂载点
tmpfs           5.4G  3.7M  5.4G    1% /run
/dev/sda2        20G   16G  3.4G   82% /
tmpfs            14G     0   14G    0% /dev/shm
none            1.0M     0  1.0M    0% /run/credentials/systemd-journald.service
tmpfs            14G  8.0K   14G    1% /tmp
none            1.0M     0  1.0M    0% /run/credentials/systemd-resolved.service
tmpfs           2.7G   88K  2.7G    1% /run/user/1000
/dev/sr0        110M  110M     0  100% /run/media/lgx/CDROM
/dev/sr1        6.1G  6.1G     0  100% /run/media/lgx/Ubuntu 26.04 amd64
tmpfs            27G     0   27G    0% /var/snap/microk8s/common/var/lib/kubelet/pods/4bcc00d7-eeb1-4f0e-a4bb-5805896636cb/volumes/kubernetes.io~secret/kubernetes-dashboard-certs
tmpfs            27G   12K   27G    1% /var/snap/microk8s/common/var/lib/kubelet/pods/4bcc00d7-eeb1-4f0e-a4bb-5805896636cb/volumes/kubernetes.io~projected/kube-api-access-nwxcd
tmpfs            27G   12K   27G    1% /var/snap/...(截断)
```

### 5. 监听端口 — ✅ 成功

```sh
(ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | head -n 80
```

```
[info] 执行远程命令: (ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | head -n 80
[info] 命令执行成功
[info] stdout: State  Recv-Q Send-Q Local Address:Port  Peer Address:PortProcess                                                                                                                                                                                                                                                                                                                                                                                                                         
LISTEN 0      4096   127.0.0.53%lo:53         0.0.0.0:*    users:(("systemd-resolve",pid=915,fd=17))                                                                                                                                                                                                                                                                                                                                                                                      
LISTEN 0      4096       127.0.0.1:43477  ...(截断)
```

### 6. 系统日志 — ✅ 成功

```sh
tail -n 50 '/var/log/syslog' 2>&1
```

```
[info] 执行远程命令: tail -n 50 '/var/log/syslog' 2>&1
[info] 命令执行成功
[info] stdout: 2026-06-11T03:09:40.940832+09:00 lgx-vmware-virtual-platform microk8s.daemon-kubelite[540799]: E0611 03:09:40.940140  540799 prober_manager.go:209] "Readiness probe already exists for container" pod="ingress/traefik-dvrzh" containerName="traefik"
2026-06-11T03:09:40.941741+09:00 lgx-vmware-virtual-platform microk8s.daemon-kubelite[540799]: E0611 03:09:40.941580  540799 pod_workers.go:1324] "Error syncing pod, skipping" err="failed to \"StartContainer\" for \"traefik\" with ImagePullBackOff: \"Back-off pulling image \\\"traefik/traefik:v3.6.2\\\": ErrImagePull: rpc error: code = NotFound desc = get image config descriptor: unexpected media type text/html for sha256:e26ea61296157f0af82987ff30cb788f3981709079fae00de5198ef17f3d20bd: not found\"" pod="ingress/traefik-dvrzh" podUID="0565ce87-8aef-475b-bb58-3bd8dc1dc57c"
2026-06-11T03:09:41.940424+09:00 lgx-vmware-virtual-platform microk8s.daemon-kubelite[540799]: E0611 03:09:41.940305  540799 prober_manager.go:209] "Readiness probe a...(截断)
```

### 7. 高内存进程 — ✅ 成功

```sh
ps aux --sort=-%mem | head -20
```

```
[info] 执行远程命令: ps aux --sort=-%mem | head -20
[info] 命令执行成功
[info] stdout: USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root      540799  8.1  1.8 1738860 529432 ?      Ssl  03:05   0:24 /snap/microk8s/8612/kubelite --scheduler-args-file=/var/snap/microk8s/8612/args/kube-scheduler --controller-manager-args-file=/var/snap/microk8s/8612/args/kube-controller-manager --proxy-args-file=/var/snap/microk8s/8612/args/kube-proxy --kubelet-args-file=/var/snap/microk8s/8612/args/kubelet --apiserver-args-file=/var/snap/microk8s/8612/args/kube-apiserver --kubeconfig-file=/var/snap/microk8s/8612/credentials/client.config --start-control-plane=true
dnsmasq    46358  0.4  1.6 2162716 474612 ?      Ssl  6月10   1:19 mysqld --character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
lgx         6407  0.1  0.9 7755896 263884 ?      Ssl  6月10   0:28 /usr/bin/gnome-shell --mode=ubuntu
lgx         8195  0.0  0.9 4034628 254672 ?      Sl   6月10   0:08 ptyxis --new-window --working-directory /home/lgx/桌面
lgx         7461  0.0  ...(截断)
```

### 8. SSH 服务状态 — ✅ 成功

```sh
systemctl status 'sshd' --no-pager 2>&1
```

```
[info] 执行远程命令: systemctl status 'sshd' --no-pager 2>&1
[info] 命令执行成功
[info] stdout: ● ssh.service - OpenBSD Secure Shell server
     Loaded: loaded (/usr/lib/systemd/system/ssh.service; disabled; preset: enabled)
     Active: active (running) since Wed 2026-06-10 22:50:23 JST; 4h 20min ago
 Invocation: 881ebb3e23a84e4a9cef19c096215f26
TriggeredBy: ● ssh.socket
       Docs: man:sshd(8)
             man:sshd_config(5)
   Main PID: 105336 (sshd)
      Tasks: 1 (limit: 28457)
     Memory: 11.8M (peak: 28.4M)
        CPU: 790ms
     CGroup: /system.slice/ssh.service
             └─105336 "sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups"

6月 11 02:37:41 lgx-vmware-virtual-platform sshd-session[488888]: Accepted password for root from 192.168.112.1 port 6380 ssh2
6月 11 02:37:41 lgx-vmware-virtual-platform sshd-session[488888]: pam_unix(sshd:session): session opened for user root(uid=0) by root(uid=0)
6月 11 02:59:23 lgx-vmware-virtual-platform sshd-session[526214]: Accepted password for root from 192.168.112.1 port 1274 ssh2
6月 11 02:59:23 lgx-...(截断)
```

### 9. Docker 容器清单 — ✅ 成功

```sh
docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>&1
```

```
[info] 执行远程命令: docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>&1
[info] 命令执行成功
[info] stdout: redis	redis	Up 4 minutes
mysql-server	mysql	Up 5 hours
```

### 10. Nginx 容器日志 — ✅ 成功

```sh
docker logs --tail 100 'nginx' 2>&1
```

```
[info] 执行远程命令: docker logs --tail 100 'nginx' 2>&1
[error] 命令执行失败，exit_code=1
[info] stdout: Error response from daemon: No such container: nginx
[warn] 命令未成功（exit_code=1），已配置为不中断工作流，继续执行
```

## 风险与建议

- **关键风险**
  - 根文件系统 `/` 使用率达到 **82%**（已用 16G / 总共 20G），剩余空间不足 4 GB，存在磁盘耗尽风险。（依据：df -h 输出 `/dev/sda2`）
  - 系统日志中 `microk8s` 持续报错，`traefik` 容器处于 `ImagePullBackOff` 状态，镜像拉取失败，可能导致 Ingress 流量入口不可用。（依据：tail /var/log/syslog 中 "ImagePullBackOff" 错误）
  - SSH 服务当前允许 **root 密码登录**，且日志已记录来自 `192.168.112.1` 的多次 root 密码登录成功事件，存在较高的暴力破解及未授权访问风险。（依据：systemctl status sshd 输出显示 `Accepted password for root`）

- **修复建议**
  - 清理磁盘空间：检查 `/var/log`、`/tmp`、Docker 镜像层、snap 旧版本等大目录，删除或转储旧文件；若业务允许可考虑扩容根分区。
  - 修复 traefik 镜像拉取问题：确认节点能否正常访问镜像仓库；检查 `microk8s` 的代理或镜像仓库配置，必要时手动拉取 `traefik/traefik:v3.6.2` 或更换为可用版本。
  - 加固 SSH 配置：编辑 `/etc/ssh/sshd_config`，将 `PermitRootLogin` 设置为 `prohibit-password`（禁止密码登录 root）或 `no`，并重启 ssh 服务；同时检查防火墙规则限制来源 IP。
  - 处理缺失的 nginx 容器：若业务依赖 nginx，需重新创建并确保其运行；若不再需要，则从巡检项中移除该检查，避免误报。

- **需要确认的问题**
  - 确认 nginx 容器是否为本环境必需组件，当前缺失是意外停止还是有意删除。
  - 确认 traefik 镜像拉取失败的根本原因（网络、仓库鉴权、镜像标签），以便决定是调整网络策略还是更换镜像源。
  - 确认根分区 82% 使用率是否已触发监控告警，以及是否有明确的清理/扩容计划。

