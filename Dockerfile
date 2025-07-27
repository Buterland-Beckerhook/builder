# Dockerfile
FROM golang:1.24-alpine AS go-build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bb-builder ./cmd/service

FROM alpine:latest AS hugo
ARG TARGETARCH
ARG HUGO_VERSION=0.148.1

RUN wget -O "hugo.tar.gz" "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${TARGETARCH}.tar.gz"
RUN tar -xf "hugo.tar.gz" hugo -C /usr/bin

FROM alpine:latest AS final

RUN apk add --no-cache --update libc6-compat libstdc++ git tzdata ca-certificates

ENV TZ=Europe/Berlin
WORKDIR /app

COPY --from=hugo /usr/bin/hugo /bin/hugo
COPY --from=go-build /build/bb-builder .

EXPOSE 8080

CMD ["./bb-builder"]
