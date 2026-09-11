#!/bin/sh
# 单容器双进程：Go 应用监听回环地址，Nginx 对外提供服务。
# 任一进程退出即整体退出，避免出现"容器活着但服务已死"的假健康状态。
set -eu

echo "[entrypoint] 启动 Go 应用：${ADDR:-127.0.0.1:8081}"
/app/server &
app_pid=$!

echo "[entrypoint] 启动 Nginx"
nginx -g 'daemon off;' &
nginx_pid=$!

shutdown() {
    echo "[entrypoint] 收到退出信号，正在关闭…"
    kill -TERM "$app_pid" 2>/dev/null || true
    kill -QUIT "$nginx_pid" 2>/dev/null || true
}

trap shutdown TERM INT

# 轮询两个进程，任意一个消失就整体退出
while kill -0 "$app_pid" 2>/dev/null && kill -0 "$nginx_pid" 2>/dev/null; do
    sleep 2
done

echo "[entrypoint] 检测到子进程退出，开始收尾"
shutdown
wait "$app_pid" 2>/dev/null || true
wait "$nginx_pid" 2>/dev/null || true
echo "[entrypoint] 已退出"
