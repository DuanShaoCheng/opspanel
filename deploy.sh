#!/bin/bash
# OpsPanel 部署脚本

set -e

echo "=== OpsPanel 部署 ==="

# 检查 docker compose
if ! command -v docker &> /dev/null; then
    echo "错误: 未安装 docker"
    exit 1
fi

# 停止旧容器
echo "停止旧容器..."
docker compose down || true

# 构建镜像
echo "构建镜像..."
docker compose build

# 启动服务
echo "启动服务..."
docker compose up -d

# 等待服务启动
echo "等待服务启动..."
sleep 3

# 检查健康状态
echo "检查服务状态..."
docker compose ps

echo ""
echo "=== 部署完成 ==="
echo "访问地址: http://localhost:$(grep LISTEN_PORT .env | cut -d= -f2)"
echo "默认账号: admin / admin123"
echo ""
echo "查看日志: docker compose logs -f"
echo "停止服务: docker compose down"
