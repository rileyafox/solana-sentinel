# internal/service/sol-ingester/DockerFile

FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v -o /out/sol-ingester ./internal/service/sol-ingester

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/sol-ingester /sol-ingester
EXPOSE 9102
ENTRYPOINT ["/sol-ingester"]
