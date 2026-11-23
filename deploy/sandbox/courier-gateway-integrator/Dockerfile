# courier-gateway-integrator 沙箱镜像：静态二进制 + alpine 基（同现有
# courier-gateway 容器惯例，见 deploy/sandbox/README.md）。
# 用法：先 go build（见 build-courier-gateway.sh），再把二进制拷进构建上下文
# 执行 docker build，或直接跑脚本。
FROM alpine:3.20
COPY courier-gateway-integrator /usr/local/bin/courier-gateway-integrator
ENTRYPOINT ["/usr/local/bin/courier-gateway-integrator"]
