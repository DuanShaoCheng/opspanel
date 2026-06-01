#!/bin/bash
# 快速部署命令 - 在 101 机器上执行

echo "=== OpsPanel 快速部署 ==="
echo ""

# 进入项目目录（根据实际路径修改）
cd /path/to/opspanel || exit 1

# 1. 备份配置
echo "[1/6] 备份配置..."
cp data/config.json data/config.json.backup.$(date +%Y%m%d_%H%M%S)

# 2. 停止服务
echo "[2/6] 停止服务..."
docker compose down

# 3. 更新代码
echo "[3/6] 更新代码..."
git pull origin master || echo "警告: git pull 失败，请手动更新代码"

# 4. 构建镜像
echo "[4/6] 构建镜像..."
docker compose build

# 5. 启动服务
echo "[5/6] 启动服务..."
docker compose up -d

# 6. 等待并检查状态
echo "[6/6] 检查服务状态..."
sleep 5
docker compose ps
echo ""

# 获取端口
PORT=$(grep LISTEN_PORT .env | cut -d= -f2)
if [ -z "$PORT" ]; then
    PORT=9091
fi

echo "=== 部署完成 ==="
echo ""
echo "⚠️  重要：需要重新配置 LLM"
echo ""
echo "1. 访问: http://$(hostname -I | awk '{print $1}'):${PORT}"
echo "2. 登录: admin / admin123"
echo "3. 进入「配置管理」添加 LLM 提供商"
echo "4. 在「日志分析」配置中选择该提供商"
echo "5. 点击「测试 LLM」验证连接"
echo ""
echo "查看日志: docker compose logs -f"
echo "详细文档: cat DEPLOY_GUIDE.md"
