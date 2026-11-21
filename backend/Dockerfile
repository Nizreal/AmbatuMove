# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH=arm64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app \
    && mkdir -p /app /data \
    && chown -R 10001:10001 /app /data
COPY --from=build /out/server /app/server
USER 10001:10001
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["serve"]
