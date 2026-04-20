FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ol1n-vault ./cmd/vault

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /data/vault
COPY --from=build /out/ol1n-vault /usr/local/bin/ol1n-vault
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/ol1n-vault"]
