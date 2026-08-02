FROM golang:1.26 AS builder

COPY . /src
WORKDIR /src/be-class_v2
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod tidy
RUN go build -o ./bin/be-class_v2 .

FROM debian:stable-slim

ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates netbase \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/be-class_v2/bin/be-class_v2 /app/be-class_v2
WORKDIR /app

EXPOSE 18000 20001 20083
VOLUME /data/conf
CMD ["./be-class_v2"]
