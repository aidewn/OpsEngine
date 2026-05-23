.PHONY: build run build-agent cross-compile clean test fmt tidy

# 编译后端服务
build:
	go build -o ops-engine.exe ./cmd/ops-engine

# 运行（开发模式）
run:
	go run ./cmd/ops-engine

# 编译 Agent（本地测试用）
build-agent:
	go build -o ops-agent.exe ./cmd/ops-agent

# 交叉编译 Agent 到 Linux
cross-compile:
	set GOOS=linux&& set GOARCH=amd64&& go build -o agents/linux_amd64/ops-agent ./cmd/ops-agent
	set GOOS=linux&& set GOARCH=arm64&& go build -o agents/linux_arm64/ops-agent ./cmd/ops-agent

# 清理编译产物
clean:
	del /f ops-engine.exe ops-agent.exe 2>nul || true
	if exist agents\linux_amd64\ops-agent del /f agents\linux_amd64\ops-agent
	if exist agents\linux_arm64\ops-agent del /f agents\linux_arm64\ops-agent

# 运行测试
test:
	go test ./...

# 代码格式化
fmt:
	go fmt ./...

# 整理依赖
tidy:
	go mod tidy
