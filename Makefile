.PHONY: dev build clean test fmt tidy

# 开发模式（热重载）
dev:
	wails dev

# 构建桌面应用
build:
	wails build

# 清理编译产物
clean:
	rm -rf build/bin

# 运行测试
test:
	go test ./...

# 代码格式化
fmt:
	go fmt ./...

# 整理依赖
tidy:
	go mod tidy
