# 评测专用镜像：单阶段官方 Go 镜像，保留完整 Go 工具链
# 供评测时在容器内改代码、编译、跑测试使用（不使用「多阶段 + 只留二进制」写法）。
FROM golang:1.22

WORKDIR /app

# 先复制依赖文件并预下载依赖（利用 Docker 缓存，也保证容器内离线可用）
COPY go.mod go.sum ./
RUN go mod download

# 复制所有项目文件
COPY . .

# 预编译一次，把编译缓存留在镜像里（不影响源码，模型仍可自由修改）
RUN go build ./...

# 容器启动后进入 shell，方便操作
CMD ["bash"]
