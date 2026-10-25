# 多阶段构建：TARGETARCH 由 docker buildx 自动注入，一份 Dockerfile 出 amd64/arm64
FROM golang:1.27-alpine AS build
ARG TARGETARCH
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /app .

FROM scratch
COPY --from=build /app /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 8080
VOLUME ["/data", "/backup"]
ENV FL_DB=/data/familyledger.db \
    FL_ADDR=:8080 \
    TZ=Asia/Shanghai
ENTRYPOINT ["/app"]
